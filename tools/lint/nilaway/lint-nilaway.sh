#!/bin/bash
go install go.uber.org/nilaway/cmd/nilaway@latest
echo linting for nil errors : https://github.com/uber-go/nilaway/?tab=readme-ov-file
~/go/bin/nilaway -json -pretty-print=false -include-pkgs="github.com/lalbers/irr" ./...
