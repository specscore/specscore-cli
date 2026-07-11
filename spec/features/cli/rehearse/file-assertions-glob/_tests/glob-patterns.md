---
status: pending
---

# Glob Patterns in File Assertions

**Verifies:** cli/rehearse/file-assertions-glob#ac:glob-multiple-match

## Setup

Create fixture files using glob patterns.

```bash
mkdir -p fixtures/data
touch fixtures/data/config1.json
touch fixtures/data/config2.json
touch fixtures/data/config3.json
```

## Assertions

### Assert: file fixtures/data/*.json exists

### Assert: file fixtures/data/*.json contains

```
config
```

### Assert: file fixtures/data/*.json permissions

```
0644
```
