#!/bin/bash

# npm run dev helper - installs air if not present
if ! command -v air &> /dev/null; then
    echo "Installing Air for live reload development..."
    go install github.com/cosmtrek/air@latest
fi

exec air