#!/bin/sh
set -e
rm -rf completions
mkdir completions
for sh in bash zsh fish powershell; do
	go run ./cmd/tiger completion "$sh" >"completions/tiger.$sh"
done
