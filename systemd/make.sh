#!/bin/bash

set -eufo pipefail
#set -x

cd "$(dirname "$0")"

SYSTEMD_DIR=/usr/lib/systemd
SYSTEMD_UNIT_DIR=$SYSTEMD_DIR/system
INPUT=coredns-dnsredir@.service.in

FILE1=$(basename $INPUT .in)
FILE2=$(echo "$FILE1" | tr -d '@')

make_files() {
    set -x
    cp $INPUT "$FILE1"
    # https://stackoverflow.com/questions/1593188/how-to-programmatically-determine-whether-the-git-checkout-is-a-tag-and-if-so-w/1593246#1593246
    TAG_OR_COMMIT=$(git name-rev --name-only --tags HEAD | sed "s/^undefined$/commit:$(git describe --dirty --always)/")
    sed -i "s/__TAG_OR_COMMIT__/$TAG_OR_COMMIT/g" "$FILE1"
    cp "$FILE1" "$FILE2"
    sed -i 's/%i/Corefile/g' "$FILE2"
    chmod 0644 "$FILE1" "$FILE2"
}

install_files() {
    set -x
    make_files
    sudo cp "$FILE1" "$FILE2" $SYSTEMD_UNIT_DIR
    sudo systemctl daemon-reload
}

uninstall_files() {
    set -x
    sudo rm -f $SYSTEMD_UNIT_DIR/"$FILE1" $SYSTEMD_UNIT_DIR/"$FILE2"
    sudo systemctl daemon-reload || true
}

clean_files() {
    set -x
    set +f
    rm -f -- *.service
    set -f
}

usage() {
cat << EOL
Usage:
    $(basename "$0") make | install | uninstall | clean

EOL
    exit 1
}

if [ $# -ne 1 ]; then
    usage
fi

CMD=$1
if [ "$CMD" == "make" ]; then
    make_files
elif [ "$CMD" == "install" ]; then
    install_files
elif [ "$CMD" == "uninstall" ]; then
    uninstall_files
elif [ "$CMD" == "clean" ]; then
    clean_files
else
    usage
fi

