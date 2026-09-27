// Package ai embeds ai/skills -- SpecScore's canonical Agent Skills --
// directly into the specscore binary.
package ai

import "embed"

// SkillsFS holds every file under skills/ at build time: skills/<name>/
// SKILL.md plus each skill's references/ subdirectories and documentation.
//
//go:embed all:skills
var SkillsFS embed.FS
