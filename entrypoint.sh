#!/bin/sh

if [ "${1#-}" != "$1" ]; then
    set -- manager "$@"
fi

if [ "$1" = "apiserver" ]; then
    shift # "apiserver"
    set -- apiserver "$@"
fi

exec "$@"
