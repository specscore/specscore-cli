// Package consilium owns the deterministic consilium engine for the
// `specscore` CLI: the vote-schema types and validator, the roster resolver
// and validator, the gate-knob and roster config loaders for the `consilium:`
// block in specscore.yaml, and the gate-rule arbiter that turns a panel's
// votes into a deterministic verdict.
//
// See `spec/features/cli/consilium/README.md` for the full Feature contract.
//
// This file currently scopes the package to the Vote type and its validator
// (REQ:vote-schema-types-and-validation). The gate engine, roster resolution,
// and config loaders land in follow-on tasks.
package consilium
