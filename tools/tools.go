// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build generate

package tools

import _ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"

//go:generate go -C .. run ./cmd/generate
//go:generate go -C .. run ./cmd/export-schema
//go:generate go -C .. run ./cmd/check-api-coverage -offline -write-doc templates/guides/api-coverage.md.tmpl
//go:generate terraform fmt -recursive ../examples/
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-dir .. -provider-name mergewf --providers-schema ../.cache/provider-schema.json
