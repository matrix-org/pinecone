#!/bin/sh

TARGET=""

while getopts "aim" option
do
    case "$option"
    in
    a) gomobile bind -v -target android -trimpath -ldflags="-s -w" github.com/matrix-org/pinecone/build/gobind ;;
    i) gomobile bind -v -target ios -trimpath -ldflags="" github.com/matrix-org/pinecone/build/gobind ;;
    m) gomobile bind -v -target macos -trimpath -ldflags="" github.com/matrix-org/pinecone/build/gobind ;;
    *) echo "No target specified, specify -a or -i"; exit 1 ;;
    esac
done