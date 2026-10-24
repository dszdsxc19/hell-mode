// Package assets ships the same skill instructions that setup exports for agent discovery.
package assets

import _ "embed"

//go:embed skills/hell-mode/SKILL.md
var Skill string

//go:embed skills/hell-challenge/SKILL.md
var ChallengeSkill string
