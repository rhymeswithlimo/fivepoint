// Package app is the Wails-bound service layer: it exposes the vault and wallet
// operations to the frontend as plain methods (available in JS as
// window.go.app.App.*) and owns the unlock/lock lifecycle.
package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/rhymeswithlimo/fivepoint/internal/chains"
	"github.com/rhymeswithlimo/fivepoint/internal/portfolio"
	"github.com/rhymeswithlimo/fivepoint/internal/prices"
	"github.com/rhymeswithlimo/fivepoint/internal/vault"
	"github.com/rhymeswithlimo/fivepoint/internal/wallet"
)

// autoLockAfter is the idle timeout before the vault locks itself.
const autoLockAfter = 5 * time.Minute

// readTimeout bounds network reads (balances/prices) for a single request.
const readTimeout = 30 * time.Second

// App is the bound application service.
type App struct {
	ctx       context.Context
	vaultPath string
	vault     *vault.Vault
	wallets   *wallet.Manager

	chains    []chains.Chain
	prices    *prices.Client
	portfolio *portfolio.Builder

	senders []chains.Chain // test-network chains used for sending
	pending *pending
}

// New constructs the app with the default vault location and chain/price clients.
// Reads use mainnet; sending uses test networks until mainnet is gated open.
func New() *App {
	chainSet := chains.Default(chains.Mainnet, chains.Config{})
	priceClient := prices.New()
	return &App{
		vaultPath: defaultVaultPath(),
		chains:    chainSet,
		prices:    priceClient,
		portfolio: portfolio.New(chainSet, priceClient),
		senders:   chains.Default(chains.Testnet, chains.Config{}),
		pending:   newPending(),
	}
}

// Startup is the Wails OnStartup hook; it captures the runtime context and loads
// any existing (locked) vault from disk.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	if vault.Exists(a.vaultPath) {
		if v, err := vault.Open(a.vaultPath, nil); err == nil {
			a.vault = v
		}
	}
}

// Status is the snapshot the frontend uses to decide which screen to show.
type Status struct {
	HasVault bool `json:"hasVault"`
	Unlocked bool `json:"unlocked"`
	HasPIN   bool `json:"hasPIN"`
}

// Status reports vault presence and lock state.
func (a *App) Status() Status {
	s := Status{HasVault: a.vault != nil || vault.Exists(a.vaultPath)}
	if a.vault != nil {
		s.Unlocked = a.vault.IsUnlocked()
		s.HasPIN = a.vault.HasPIN()
	}
	return s
}

// CreateVault initializes a new vault with the master passphrase and leaves it
// unlocked.
func (a *App) CreateVault(passphrase string) error {
	if a.vault != nil && vault.Exists(a.vaultPath) {
		return errors.New("a vault already exists")
	}
	v, err := vault.Create(a.vaultPath, passphrase, nil)
	if err != nil {
		return err
	}
	a.bind(v)
	return nil
}

// Unlock opens the vault with the master passphrase.
func (a *App) Unlock(passphrase string) error {
	if err := a.ensureVault(); err != nil {
		return err
	}
	if err := a.vault.UnlockWithPassphrase(passphrase); err != nil {
		return err
	}
	a.afterUnlock()
	return nil
}

// UnlockPIN opens the vault with the convenience PIN.
func (a *App) UnlockPIN(pin string) error {
	if err := a.ensureVault(); err != nil {
		return err
	}
	if err := a.vault.UnlockWithPIN(pin); err != nil {
		return err
	}
	a.afterUnlock()
	return nil
}

// Lock clears in-memory secrets.
func (a *App) Lock() {
	if a.vault != nil {
		a.vault.Lock()
	}
}

// SetPIN configures the convenience PIN (vault must be unlocked).
func (a *App) SetPIN(pin string) error {
	if a.vault == nil {
		return vault.ErrLocked
	}
	return a.vault.SetPIN(pin)
}

// CreateResult carries a new wallet's metadata plus its one-time mnemonic for
// the backup screen.
type CreateResult struct {
	Wallet   wallet.Info `json:"wallet"`
	Mnemonic string      `json:"mnemonic"`
}

// CreateWallet generates a new HD wallet.
func (a *App) CreateWallet(name, icon, color string) (CreateResult, error) {
	if a.wallets == nil {
		return CreateResult{}, vault.ErrLocked
	}
	info, mnemonic, err := a.wallets.Create(name, icon, color, wallet.Strength12)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Wallet: info, Mnemonic: mnemonic}, nil
}

// ImportMnemonic adds an existing seed-phrase wallet.
func (a *App) ImportMnemonic(name, icon, color, mnemonic, passphrase string) (wallet.Info, error) {
	if a.wallets == nil {
		return wallet.Info{}, vault.ErrLocked
	}
	return a.wallets.ImportMnemonic(name, icon, color, mnemonic, passphrase)
}

// ImportPrivateKey adds a single-key wallet.
func (a *App) ImportPrivateKey(name, icon, color, key string) (wallet.Info, error) {
	if a.wallets == nil {
		return wallet.Info{}, vault.ErrLocked
	}
	return a.wallets.ImportPrivateKey(name, icon, color, key)
}

// ListWallets returns metadata for all wallets (never secrets).
func (a *App) ListWallets() ([]wallet.Info, error) {
	if a.wallets == nil {
		return nil, vault.ErrLocked
	}
	return a.wallets.List()
}

// WalletValue is a wallet's metadata plus its current USD value, for the home
// grid.
type WalletValue struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Icon  string  `json:"icon"`
	Color string  `json:"color"`
	USD   float64 `json:"usd"`
}

// Overview is the aggregate across all wallets, for the home screen.
type Overview struct {
	TotalUSD    float64       `json:"totalUsd"`
	Change1d    float64       `json:"change1d"`
	ChangeUSD1d float64       `json:"changeUsd1d"`
	Wallets     []WalletValue `json:"wallets"`
}

// Overview returns the total portfolio value across all wallets plus a per-wallet
// breakdown. Network reads happen per wallet; prices are cached across them.
func (a *App) Overview() (Overview, error) {
	if a.wallets == nil {
		return Overview{}, vault.ErrLocked
	}
	infos, err := a.wallets.List()
	if err != nil {
		return Overview{}, err
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), readTimeout)
	defer cancel()

	var ov Overview
	for _, info := range infos {
		wv := WalletValue{ID: info.ID, Name: info.Name, Icon: info.Icon, Color: info.Color}
		if sum, err := a.summaryFor(ctx, info.ID); err == nil {
			wv.USD = sum.TotalUSD
			ov.TotalUSD += sum.TotalUSD
			ov.ChangeUSD1d += sum.ChangeUSD1d
		}
		ov.Wallets = append(ov.Wallets, wv)
	}
	if ov.TotalUSD > 0 {
		ov.Change1d = ov.ChangeUSD1d / ov.TotalUSD * 100
	}
	return ov, nil
}

// WalletPortfolio returns the valued, per-asset portfolio for one wallet.
func (a *App) WalletPortfolio(id string) (portfolio.Summary, error) {
	if a.wallets == nil {
		return portfolio.Summary{}, vault.ErrLocked
	}
	ctx, cancel := context.WithTimeout(a.baseCtx(), readTimeout)
	defer cancel()
	return a.summaryFor(ctx, id)
}

// summaryFor derives the seed for a wallet and builds its portfolio. Private-key
// wallets (no HD seed) yield an empty portfolio rather than an error.
func (a *App) summaryFor(ctx context.Context, id string) (portfolio.Summary, error) {
	seed, err := a.wallets.Seed(id)
	if errors.Is(err, wallet.ErrNoSeed) {
		return portfolio.Summary{}, nil
	}
	if err != nil {
		return portfolio.Summary{}, err
	}
	defer zeroBytes(seed)
	return a.portfolio.ForSeed(ctx, seed)
}

func (a *App) baseCtx() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// RevealSeed returns a wallet's mnemonic/key for backup, re-authenticating with
// the passphrase first so a left-open session can't dump secrets.
func (a *App) RevealSeed(id, passphrase string) (string, error) {
	if a.vault == nil || a.wallets == nil {
		return "", vault.ErrLocked
	}
	if !a.vault.VerifyPassphrase(passphrase) {
		return "", vault.ErrWrongPassphrase
	}
	return a.wallets.RevealSeed(id)
}

func (a *App) ensureVault() error {
	if a.vault != nil {
		return nil
	}
	v, err := vault.Open(a.vaultPath, nil)
	if err != nil {
		return err
	}
	a.vault = v
	return nil
}

func (a *App) bind(v *vault.Vault) {
	a.vault = v
	a.afterUnlock()
}

func (a *App) afterUnlock() {
	a.wallets = wallet.NewManager(a.vault)
	a.vault.SetAutoLock(autoLockAfter, func() { a.wallets = nil })
}

// defaultVaultPath returns the OS-appropriate location for the vault file, e.g.
// %AppData%\Fivepoint\vault.json on Windows.
func defaultVaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "Fivepoint", "vault.json")
}
