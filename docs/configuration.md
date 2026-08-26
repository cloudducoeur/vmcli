# Configuration

`vmcli` reads `VMCLI_CONFIG`, `--config`, or `~/.config/vmcli/config.yaml`.

```yaml
refreshInterval: 5s
clusters:
  production:
    vminsert:
      - https://vminsert01.example.org:8480
    vmselect:
      - https://vmselect01.example.org:8481
    vmstorage:
      - https://vmstorage01.example.org:8482
    vmalert:
      - https://vmalert.example.org
    tenant:
      accountID: 0
      projectID: 0
    auth:
      username: admin
      password: secret
      # bearerToken: set this instead of basic auth when appropriate
    tls:
      caFile: /path/to/ca.pem
      insecureSkipVerify: false
```

Credentials are used only by the HTTP client and are not rendered by the TUI.

`tls` settings apply only when at least one endpoint uses `https://`. If all
endpoints are `http://`, the `tls` block is ignored. When
`insecureSkipVerify: true`, CA file loading is skipped and certificate
verification is disabled.
