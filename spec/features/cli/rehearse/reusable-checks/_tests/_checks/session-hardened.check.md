# Check: session-hardened

**Params:** response_file

```bash
grep -qi '^set-cookie: session=' {{response_file}} || { echo "no session cookie"; exit 1; }
grep -i '^set-cookie: session=' {{response_file}} | grep -qi 'HttpOnly' || { echo "cookie not HttpOnly"; exit 1; }
echo "session hardened"
```
