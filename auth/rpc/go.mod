module github.com/keys-pub/keys-ext/auth/rpc

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.1.2
	github.com/keys-pub/go-libfido2 v1.4.1-0.20200914200216-8e225f5fe92e
	github.com/keys-pub/keys-ext/auth/fido2 v0.0.0-20201017235523-afa75b9040f2
	github.com/kr/pretty v0.1.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.5.1
	golang.org/x/net v0.0.0-20200904194848-62affa334b73 // indirect
	golang.org/x/sys v0.0.0-20200917073148-efd3b9a0ff20 // indirect
	google.golang.org/genproto v0.0.0-20200917134801-bb4cff56e0d0 // indirect
	google.golang.org/grpc v1.32.0
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)

// replace github.com/keys-pub/keys-ext/auth/fido2 => ../fido2

// replace github.com/keys-pub/go-libfido2 => ../../../go-libfido2
