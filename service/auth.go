package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"

	"github.com/keys-pub/keys"
	"github.com/keys-pub/keys/keyring"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh/terminal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TODO: Some clients log grpc requests which for AuthSetup and AuthUnlock include a password.
//       We need to ensure client logging doesn't do this, or use an alternate
//       channel for auth? (Also redux state in frontend could keep password around.)
//       Maybe a special password proto type that can't be logged and clears
//       itself?

type auth struct {
	sync.Mutex
	cfg       *Config
	keyring   keyring.Keyring
	tokens    map[string]string
	whitelist *keys.StringSet
}

func newAuth(cfg *Config) (*auth, error) {
	// We don't need auth for the following methods.
	whitelist := keys.NewStringSet(
		"/service.Keys/AuthGenerate",
		"/service.Keys/AuthSetup",
		"/service.Keys/AuthUnlock",
		"/service.Keys/AuthLock",
		"/service.Keys/RuntimeStatus")

	kr, err := newKeyring(cfg)
	if err != nil {
		return nil, err
	}

	return &auth{
		cfg:       cfg,
		keyring:   kr,
		tokens:    map[string]string{},
		whitelist: whitelist,
	}, nil
}

func newKeyring(cfg *Config) (keyring.Keyring, error) {
	switch cfg.KeyringType() {
	case KeyringTypeDefault:
		logger.Infof("Keyring: default")
		return keyring.NewKeyring(cfg.AppName())
	case KeyringTypeFS:
		logger.Infof("Keyring: fs")
		dir, err := cfg.AppPath("keyring", false)
		if err != nil {
			return nil, err
		}
		return keyring.NewFS(dir)
	case KeyringTypeMem:
		logger.Infof("Keyring: mem")
		return keyring.NewMem(), nil
	default:
		return nil, errors.Errorf("unknown keyring type")
	}
}

func (a *auth) lock() {
	// TODO: Lock after running for a certain amount of time (maybe a few hours?)
	logger.Infof("Locking")
	a.keyring.Lock()
	a.tokens = map[string]string{}
}

func (a *auth) unlock(password string, client string) (string, error) {
	logger.Infof("Unlock")
	salt, err := a.keyring.Salt()
	if err != nil {
		return "", errors.Wrapf(err, "failed to load salt")
	}
	auth, err := keyring.NewPasswordAuth(password, salt)
	if err != nil {
		return "", err
	}
	if err := a.keyring.Unlock(auth); err != nil {
		if err == keyring.ErrInvalidAuth {
			return "", status.Error(codes.PermissionDenied, "invalid password")
		}
		return "", err
	}

	token := generateToken()
	a.tokens[client] = token
	logger.Infof("Unlocked")

	return token, nil
}

func generateToken() string {
	return keys.MustEncode(keys.Rand32()[:], keys.Base62)
}

func (a *auth) streamInterceptor(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := a.authorize(stream.Context(), info.FullMethod); err != nil {
		return err
	}
	return handler(srv, stream)
}

func (a *auth) unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if err := a.authorize(ctx, info.FullMethod); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func (a *auth) authorize(ctx context.Context, method string) error {
	// No authorization needed for whitelisted methods.
	if a.whitelist.Contains(method) {
		logger.Infof("Authorization is not required for %s", method)
		return nil
	}

	logger.Infof("Authorize %s", method)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if len(md["authorization"]) == 0 {
			logger.Warningf("Auth token missing from request")
			return status.Error(codes.PermissionDenied, "authorization missing")
		}
		token := md["authorization"][0]
		for _, t := range a.tokens {
			if t == token {
				return nil
			}
		}

		logger.Infof("Invalid auth token")
		return status.Error(codes.PermissionDenied, "invalid token")
	}
	return status.Error(codes.PermissionDenied, "no authorization in context")
}

type clientAuth struct {
	token string
}

func newClientAuth(token string) clientAuth {
	return clientAuth{token: token}
}

func (a clientAuth) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	if a.token == "" {
		return nil, nil
	}
	return map[string]string{
		"authorization": a.token,
	}, nil
}

func (a clientAuth) RequireTransportSecurity() bool {
	return true
}

func (s *service) isAuthSetupNeeded() (bool, error) {
	kr := s.ks.Keyring()
	if kr == nil {
		return false, errors.Errorf("no keyring set")
	}
	authed, err := kr.Authed()
	if err != nil {
		return false, err
	}
	return !authed, nil
}

// AuthSetup (RPC) ...
func (s *service) AuthSetup(ctx context.Context, req *AuthSetupRequest) (*AuthSetupResponse, error) {
	setupNeeded, err := s.isAuthSetupNeeded()
	if err != nil {
		return nil, err
	}
	if !setupNeeded {
		return nil, errors.Errorf("auth already setup")
	}

	token, err := s.auth.unlock(req.Password, req.Client)
	if err != nil {
		return nil, err
	}

	if err := s.Open(); err != nil {
		return nil, err
	}

	return &AuthSetupResponse{
		AuthToken: token,
	}, nil
}

// AuthUnlock (RPC) ...
func (s *service) AuthUnlock(ctx context.Context, req *AuthUnlockRequest) (*AuthUnlockResponse, error) {
	if req.Password == "" {
		return nil, errors.Errorf("no password specified")
	}

	setupNeeded, err := s.isAuthSetupNeeded()
	if err != nil {
		return nil, err
	}
	if setupNeeded {
		return nil, errors.Errorf("auth setup needed")
	}

	token, err := s.auth.unlock(req.Password, req.Client)
	if err != nil {
		return nil, err
	}

	if err := s.Open(); err != nil {
		return nil, err
	}

	return &AuthUnlockResponse{
		AuthToken: token,
	}, nil
}

// AuthLock (RPC) ...
func (s *service) AuthLock(ctx context.Context, req *AuthLockRequest) (*AuthLockResponse, error) {
	s.auth.lock()

	s.Close()

	return &AuthLockResponse{}, nil
}

func readPassword(label string, verify bool) ([]byte, error) {
	if !terminal.IsTerminal(int(syscall.Stdin)) {
		return nil, errors.Errorf("failed to read password from terminal: not a terminal or terminal not supported; use --password option when calling `keys auth`.")
	}
	fmt.Fprintf(os.Stderr, label)
	password, err := terminal.ReadPassword(int(syscall.Stdin))
	fmt.Fprintf(os.Stderr, "\n")
	if err != nil {
		return nil, err
	}
	if len(password) == 0 {
		return nil, errors.Errorf("no password")
	}
	if len(password) < 10 {
		return nil, errors.Errorf("password is too short")
	}

	if verify {
		fmt.Fprintf(os.Stderr, "Re-enter the password:")
		password2, err := terminal.ReadPassword(int(syscall.Stdin))
		fmt.Fprintf(os.Stderr, "\n")
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(password, password2) {
			return nil, errors.Errorf("passwords don't match")
		}
	}

	return password, nil
}

type testClientAuth struct {
	token string
}

func newTestClientAuth(token string) testClientAuth {
	return testClientAuth{token: token}
}

func (a testClientAuth) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	if a.token == "" {
		return nil, nil
	}
	return map[string]string{
		"authorization": a.token,
	}, nil
}

func (a testClientAuth) RequireTransportSecurity() bool {
	// For test client
	return false
}
