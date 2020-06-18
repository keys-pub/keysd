package service

import (
	"context"
	"sync"

	"github.com/keys-pub/keys"
	"github.com/keys-pub/keys-ext/auth/fido2"
	"github.com/keys-pub/keys-ext/vault"
	"github.com/keys-pub/keys/ds"
	"github.com/keys-pub/keys/encoding"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TODO: Some clients could log grpc requests which for AuthSetup and AuthUnlock include a password.
//       We need to ensure client logging doesn't do this in the future accidentally.

type auth struct {
	sync.Mutex
	cfg       *Config
	tokens    map[string]string
	allowlist *ds.StringSet

	fas fido2.AuthServer
}

func newAuth(cfg *Config) *auth {
	// We don't need auth for the following methods.
	allowlist := ds.NewStringSet(
		"/service.Keys/AuthSetup",
		"/service.Keys/AuthUnlock",
		"/service.Keys/AuthLock",
		"/service.Keys/RuntimeStatus",
		"/service.Keys/Rand",
		"/service.Keys/RandPassword",
		"/service.Keys/Restore",
	)

	return &auth{
		cfg:       cfg,
		tokens:    map[string]string{},
		allowlist: allowlist,
	}
}

func (a *auth) reset() {
	a.tokens = map[string]string{}
}

func (a *auth) setup(ctx context.Context, vlt *vault.Vault, secret string, typ AuthType) error {
	logger.Infof("Setup (%s)", typ)
	switch typ {
	case PasswordAuth:
		if err := setupPassword(vlt, secret); err != nil {
			return authErr(err, typ, "failed to setup")
		}
		return nil
	case FIDO2HMACSecretAuth:
		_, err := setupHMACSecret(ctx, a.fas, vlt, secret, a.cfg.AppName())
		if err != nil {
			return authErr(err, typ, "failed to setup")
		}
		return nil
	default:
		return errors.Errorf("unsupported auth type")
	}
}

func (a *auth) unlock(ctx context.Context, vlt *vault.Vault, secret string, typ AuthType, client string) (string, error) {
	logger.Infof("Unlock (%s)", typ)

	switch typ {
	case PasswordAuth:
		if _, err := unlockPassword(vlt, secret); err != nil {
			return "", authErr(err, typ, "failed to unlock")
		}
	case FIDO2HMACSecretAuth:
		if err := unlockHMACSecret(ctx, a.fas, vlt, secret); err != nil {
			return "", authErr(err, typ, "failed to unlock")
		}
	default:
		return "", errors.Errorf("unsupported auth type")
	}

	logger.Infof("Unlocked (%s)", typ)
	token := a.registerToken(client)
	return token, nil
}

func (a *auth) provision(ctx context.Context, vlt *vault.Vault, secret string, typ AuthType, setup bool) (*vault.Provision, error) {
	logger.Infof("Provision (%s)", typ)
	switch typ {
	case PasswordAuth:
		return provisionPassword(ctx, vlt, secret)
	case FIDO2HMACSecretAuth:
		if setup {
			return setupHMACSecret(ctx, a.fas, vlt, secret, a.cfg.AppName())
		}
		return provisionHMACSecret(ctx, a.fas, vlt, secret)
	default:
		return nil, errors.Errorf("unknown auth type")
	}
}

func (a *auth) registerToken(client string) string {
	token := generateToken()
	a.tokens[client] = token
	return token
}

func authErr(err error, typ AuthType, wrap string) error {
	if errors.Cause(err) == vault.ErrInvalidAuth {
		switch typ {
		case PasswordAuth:
			return status.Error(codes.Unauthenticated, "invalid password")
		default:
			return status.Error(codes.Unauthenticated, "invalid auth")
		}

	}
	return errors.Wrapf(err, wrap)
}

func generateToken() string {
	return encoding.MustEncode(keys.Rand32()[:], encoding.Base62)
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
	// No authorization needed for allowed methods.
	if a.allowlist.Contains(method) {
		logger.Infof("Authorization is not required for %s", method)
		return nil
	}

	logger.Infof("Authorize %s", method)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if len(md["authorization"]) == 0 {
			logger.Warningf("Auth token missing from request")
			return status.Error(codes.Unauthenticated, "authorization missing")
		}
		token := md["authorization"][0]
		for _, t := range a.tokens {
			if t == token {
				return nil
			}
		}

		logger.Infof("Invalid auth token")
		return status.Error(codes.Unauthenticated, "invalid token")
	}
	return status.Error(codes.Unauthenticated, "no authorization in context")
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
