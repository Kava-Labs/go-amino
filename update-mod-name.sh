#! /bin/bash
set -e

# update all self-imports in the module to the new module name
grep -rl --include=*.go \"github.com/tendermint/go-amino . | xargs sed -i '' 's#"github.com/tendermint/go-amino#"github.com/kava-labs/go-amino#'

# add a go mod file (auto migrates from dep)
go mod init github.com/kava-labs/go-amino
go mod tidy

# remove no longer needed dep files
rm Gopkg.lock Gopkg.toml