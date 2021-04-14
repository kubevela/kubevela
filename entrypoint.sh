#!/bin/sh

if [[ "${1#-}" != "$1" ]]; then
    set -- manager "$@"
fi

exec "$@"
