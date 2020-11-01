#!/bin/bash

set -euf
#set -x

if [ "$(uname -s)" != "Darwin" ]; then
    echo "This script only available on macOS"
    exit 1
fi

cd "$(dirname "$0")"
DIR=/usr/local/include

echo Downloading pf headers into /usr/local/include ...

mkdir -p "$DIR/net"
curl -sL -o "$DIR/net/pfvar.h" https://raw.githubusercontent.com/apple/darwin-xnu/master/bsd/net/pfvar.h
curl -sL -o "$DIR/net/radix.h" https://raw.githubusercontent.com/apple/darwin-xnu/master/bsd/net/radix.h

mkdir -p "$DIR/libkern"
curl -sL -o "$DIR/libkern/tree.h" https://raw.githubusercontent.com/apple/darwin-xnu/master/libkern/libkern/tree.h

echo Done

