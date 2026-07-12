# Check: redirected-to-dashboard

**Params:** response_file, location

```bash
grep -qi '^HTTP/[0-9.]* 302' {{response_file}} || { echo "not a 302 redirect"; exit 1; }
grep -qi "^Location: {{location}}" {{response_file}} || { echo "not redirected to {{location}}"; exit 1; }
echo "redirected to {{location}}"
```
