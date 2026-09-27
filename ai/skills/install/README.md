# specscore Install Skill

Install / reinstall / update the [`specscore`](https://github.com/synchestra-io/specscore) CLI. Implementation lives in [`SKILL.md`](SKILL.md); the `/specscore:install` slash-command alias is wired up via [`commands/install.md`](../../commands/install.md). Design rationale and explicit non-goals live in [`docs/ideas/cli-install-skill.md`](../../docs/ideas/cli-install-skill.md).

## Sister skill

This skill mirrors [`synchestra:install`](https://github.com/synchestra-io/ai-plugin-synchestra/blob/main/skills/install/SKILL.md) in the sibling [`ai-plugin-synchestra`](https://github.com/synchestra-io/ai-plugin-synchestra) plugin.

**The two implementations should remain similar in shape.** Both follow the same template:

| Concern | Same in both |
|---|---|
| File layout | `skills/install/SKILL.md` + `commands/install.md` + `docs/ideas/cli-install-skill.md` + this README |
| Behavior | Show the user the `curl -fsSL https://<domain>/get-cli \| sh` one-liner and the manual install path; **do not** execute `curl \| sh` from inside the skill |
| Verification | Run `<cli> --version` after the user confirms install is complete |
| Tone & tense | "What to show the user", "Behavior", "Verification" sections; same wording where the only difference is the CLI name |

When changing one, consider whether the same change should land in the other. Substantive divergence (e.g., one CLI gains a meaningfully different install method) is fine — but cosmetic drift is noise.

## What's different between the two

Only what must be:

- CLI name: `specscore` ↔ `synchestra`
- Install URLs: `https://specscore.md/get-cli` ↔ `https://synchestra.io/get-cli`; `https://specscore.md/install` ↔ `https://synchestra.io/install`
- Repo references: `synchestra-io/specscore` ↔ `synchestra-io/synchestra`
- `cli/version` feature link points at the respective CLI repo's spec tree

Anything else that diverges is a candidate for review.
