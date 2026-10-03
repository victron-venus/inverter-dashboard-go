#!/bin/bash

# npm run dev helper - installs air if not present
if ! command -v air &> /dev/null; then
    echo "Installing Air for live reload development..."
    go install github.com/air-verse/air@9f19e52511f7bb697036e6ee2c12a212e742199d # v1.67.4
fi

exec air
