#!/bin/bash

set -euf
#set -x

cd "$(dirname "$0")"

SYSTEMD_DIR=/usr/lib/systemd
SYSTEMD_UNIT_DIR=$SYSTEMD_DIR/system
INPUT=coredns-dnsredir@.service.in

FILE1=$(basename $INPUT .in)
FILE2=$(echo "$FILE1" | tr -d '@')

install_files() {
    set -x
    cp $INPUT "$FILE1"
    cp $INPUT "$FILE2"
    sed -i 's/%i/Corefile/g' "$FILE2"
    chmod 0644 "$FILE1" "$FILE2"
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
    rm -f *.service
}

usage() {
cat << EOL
Usage:
    $(basename "$0") install | uninstall | clean

EOL
    exit 1
}

if [ $# -ne 1 ]; then
    usage
fi

CMD=$1
if [ "$CMD" == "install" ]; then
    install_files
elif [ "$CMD" == "uninstall" ]; then
    uninstall_files
elif [ "$CMD" == "clean" ]; then
    clean_files
else
    usage
fi

