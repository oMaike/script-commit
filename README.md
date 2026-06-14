# Daily Commit Watchdog em Go

Automacao para manter o repositorio privado com commits frequentes sem deixar passar 24 horas entre um commit e outro.

O workflow roda a cada hora, mas o script Go so cria commit quando o ultimo commit do repositorio ja tem pelo menos `16` horas. Isso evita um commit por hora e ainda deixa uma margem antes das 24 horas.

## Como instalar no repositorio `oMaike/script-commit`

1. Copie as pastas `.github` e `scripts` deste diretorio para a raiz do repositorio.
2. Faca commit e push desses arquivos no GitHub.
3. No GitHub, abra `Settings > Actions > General`.
4. Em `Workflow permissions`, selecione `Read and write permissions`.
5. Abra `Actions > Daily commit watchdog > Run workflow`, marque `force_commit` e execute uma vez para testar.

## Teste no Windows

Depois de instalar Git e Go, abra o `cmd` na raiz do repositorio e rode:

```bat
test_now.cmd
```

Esse teste cria commit local e nao faz push porque usa `SKIP_PUSH=true`.

Para rodar sem forcar commit, use:

```bat
run_watchdog.cmd
```

## Configuracao

No arquivo `.github/workflows/daily-commit.yml`, ajuste:

- `MIN_HOURS_BETWEEN_COMMITS`: use `16` para boa margem antes de 24h.
- `HEARTBEAT_FILE`: arquivo que sera atualizado pelo commit automatico.
- `cron`: horario/frequencia do watchdog. O valor atual, `17 * * * *`, roda uma vez por hora.
- `go-version`: versao do Go usada pelo GitHub Actions.

## Observacao importante

GitHub Actions agenda workflows com boa confiabilidade, mas nao oferece garantia absoluta de execucao no minuto exato. Rodar de hora em hora e commitar depois de 16h reduz bastante o risco de passar de 24h. Para garantia mais rigida, use o mesmo `scripts/daily_commit.go` em um servidor/runner proprio com cron ou systemd timer.
