#!/usr/bin/env bash

set -e -u -o pipefail # Fail on error

dir=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )
cd $dir

tmpdir=`mktemp -d 2>/dev/null || mktemp -d -t 'mytmpdir'`

bin=$1
pkg=$2
build_only=${BUILD_ONLY:-""}
dest=${DEST:-"$HOME/go/bin"}
debug=${DEBUG:-"0"}
version=${VERSION:-"0.0.0-dev"}
date=${DATE:-""}

echoerr() { 
    if [ "$debug" = "1" ]; then
        printf "%s\n" "$*" >&2;
    fi
}

commit=`git rev-parse --short HEAD`

echo "Using go at `which go`"
echo "Go version: `go version`"

# Build
echoerr "Building $bin from $pkg ($version $commit $date)"
(cd "$pkg" && go build -ldflags "-X main.version=$version -X main.commit=$commit -X main.date=$date" . && mv $bin "$tmpdir")

# Codesign
if [[ "$OSTYPE" == "darwin"* ]]; then
    code_sign_identity="Developer ID Application: Gabriel Handford (U2622K69A6)"
    echoerr "Signing: $code_sign_identity"
    echoerr $(codesign --verbose --sign "$code_sign_identity" "$tmpdir/$bin" 2>&1)
else
    echoerr "No codesign for OSTYPE=$OSTYPE"
fi

# Copy to dest
mkdir -p "$dest"
echoerr "Installing at $dest/$bin"
mv "$tmpdir/$bin" "$dest/$bin"

if [ ! "$build_only" = "1" ]; then
    cd "$dest"
    ./$bin "$@"
fi
