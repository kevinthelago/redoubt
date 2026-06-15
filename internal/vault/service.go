package vault

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/kardianos/service"
)

const (
	serviceName        = "RedoubtVault"
	serviceDisplayName = "Redoubt Vault Server"
	serviceDescription = "Append-only, LAN-only backup vault (Redoubt). Binds to a private RFC1918 address only; no WAN listener."
)

// ServiceConfig holds the parameters needed to run the rest-server process.
type ServiceConfig struct {
	// ListenAddr is the validated RFC1918 address (host:port).
	ListenAddr string
	// RepoDir is the filesystem path of the restic repository.
	RepoDir string
	// TLSCertPath is the path to the server TLS certificate (PEM).
	TLSCertPath string
	// TLSKeyPath is the path to the server TLS private key (PEM).
	TLSKeyPath string
	// RestServerBin is the absolute path to the rest-server binary.
	RestServerBin string
}

// buildServerArgs returns the command-line arguments for rest-server.
// --append-only is ALWAYS present: this is the invariant that makes the vault
// ransomware-resistant. Tests verify it is structural and cannot be dropped.
func buildServerArgs(cfg ServiceConfig) []string {
	return []string{
		"--listen", cfg.ListenAddr,
		"--path", cfg.RepoDir,
		"--append-only", // INVARIANT: never remove — prevents client-side history deletion
		"--tls",
		"--tls-cert", cfg.TLSCertPath,
		"--tls-key", cfg.TLSKeyPath,
		"--no-auth",
	}
}

// vaultSvc implements service.Interface for kardianos/service.
type vaultSvc struct {
	cfg ServiceConfig
	mu  sync.Mutex
	cmd *exec.Cmd
}

func (vs *vaultSvc) Start(s service.Service) error {
	go vs.run()
	return nil
}

func (vs *vaultSvc) Stop(s service.Service) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.cmd != nil && vs.cmd.Process != nil {
		return vs.cmd.Process.Kill()
	}
	return nil
}

func (vs *vaultSvc) run() {
	vs.mu.Lock()
	args := buildServerArgs(vs.cfg)
	vs.cmd = exec.Command(vs.cfg.RestServerBin, args...)
	vs.cmd.Stdout = os.Stdout
	vs.cmd.Stderr = os.Stderr
	vs.mu.Unlock()

	_ = vs.cmd.Run()
}

// InstallService registers the vault service with the OS service manager.
// vaultBase is the directory containing vault.json — passed as --vault-base to the service.
func InstallService(cfg ServiceConfig, vaultBase string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	svcCfg := &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
		Executable:  exe,
		Arguments:   []string{"vault", "serve", "--vault-base", vaultBase},
	}

	vs := &vaultSvc{cfg: cfg}
	svc, err := service.New(vs, svcCfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	if err := svc.Install(); err != nil {
		return fmt.Errorf("install service %q: %w", serviceName, err)
	}
	return nil
}

// StartService starts the already-installed vault OS service.
func StartService() error {
	vs := &vaultSvc{}
	svcCfg := &service.Config{Name: serviceName}
	svc, err := service.New(vs, svcCfg)
	if err != nil {
		return fmt.Errorf("create service handle: %w", err)
	}
	if err := svc.Start(); err != nil {
		return fmt.Errorf("start service %q: %w", serviceName, err)
	}
	return nil
}

// UninstallService removes the vault service from the OS service manager.
func UninstallService() error {
	vs := &vaultSvc{}
	svcCfg := &service.Config{Name: serviceName}
	svc, err := service.New(vs, svcCfg)
	if err != nil {
		return fmt.Errorf("create service handle: %w", err)
	}
	if err := svc.Uninstall(); err != nil {
		return fmt.Errorf("uninstall service %q: %w", serviceName, err)
	}
	return nil
}

// RunService is the service entrypoint called by "redoubt vault serve".
// It blocks until the service is stopped by the OS service manager.
func RunService(cfg ServiceConfig) error {
	vs := &vaultSvc{cfg: cfg}
	svcCfg := &service.Config{
		Name:        serviceName,
		DisplayName: serviceDisplayName,
		Description: serviceDescription,
	}
	svc, err := service.New(vs, svcCfg)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	return svc.Run()
}

// lookupRestServer finds the rest-server binary on PATH.
func lookupRestServer() (string, error) {
	path, err := exec.LookPath("rest-server")
	if err != nil {
		return "", ErrRestServerNotFound
	}
	return path, nil
}
