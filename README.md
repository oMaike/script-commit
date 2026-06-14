# Daily Commit Watchdog em Go

Automacao para manter o repositorio privado com commits frequentes sem deixar passar 24 horas entre um commit e outro.

O workflow roda a cada hora, mas o script Go so cria commit quando o ultimo commit do repositorio ja tem pelo menos `23` horas. Isso evita um commit por hora e mantem uma palavra por commit diario sem deixar passar 24 horas.

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

## Roteiro palavra por palavra

O script monta o arquivo `roteiro.md` com uma palavra por commit.

Ele usa o arquivo local `meu roteiro.txt` como fonte. A cada execucao valida, o script pega a proxima palavra desse arquivo, acrescenta em `roteiro.md`, atualiza `.daily-commit/word-state.json` e cria um commit.

Arquivos usados:

- `meu roteiro.txt`: texto-fonte local extraido do seu JS.
- `roteiro.md`: arquivo gerado, uma palavra por vez.
- `.daily-commit/word-state.json`: indice da proxima palavra.

Para converter novamente um JS no mesmo formato:

```powershell
$jsPath = "meu roteiro.js"
$txtPath = "meu roteiro.txt"
$text = [System.IO.File]::ReadAllText($jsPath, [System.Text.Encoding]::UTF8)
$startToken = "enviarScript(`"
$start = $text.IndexOf($startToken) + $startToken.Length
$end = $text.LastIndexOf("`).then")
$body = $text.Substring($start, $end - $start).Trim()
[System.IO.File]::WriteAllText($txtPath, $body, [System.Text.Encoding]::UTF8)
test_now.cmd
```

Para reiniciar do zero:

```bash
rm -f roteiro.md .daily-commit/word-state.json
```

## Rodar 24/7 em uma VPS

O script nao precisa ficar aberto em loop. O ideal e deixar a VPS ligada 24/7 e chamar o script de hora em hora com `cron`. Como `MIN_HOURS_BETWEEN_COMMITS=23`, ele so cria commit quando o ultimo commit tiver pelo menos 23 horas.

### 1. Instale Git e Go

Em Ubuntu/Debian:

```bash
sudo apt update
sudo apt install -y git golang-go
```

Confira:

```bash
git --version
go version
```

### 2. Clone o repositorio privado

Recomendado com SSH:

```bash
git clone git@github.com:oMaike/script-commit.git
cd script-commit
```

Se a VPS ainda nao tiver chave SSH:

```bash
ssh-keygen -t ed25519 -C "vps-script-commit"
cat ~/.ssh/id_ed25519.pub
```

Adicione a chave publica no GitHub em `Settings > Deploy keys` do repositorio e marque `Allow write access`.

### 3. Configure o autor dos commits

Dentro do repositorio:

```bash
git config user.name "oMaike Bot"
git config user.email "oMaike@users.noreply.github.com"
```

### 4. Teste manualmente com push real

```bash
MIN_HOURS_BETWEEN_COMMITS=23 \
HEARTBEAT_FILE=.daily-commit/heartbeat.json \
SOURCE_TEXT_FILE="meu roteiro.txt" \
OUTPUT_TEXT_FILE=roteiro.md \
WORD_STATE_FILE=.daily-commit/word-state.json \
TARGET_BRANCH=main \
FORCE_COMMIT=true \
SKIP_PUSH=false \
go run ./scripts/daily_commit.go
```

Se esse comando criar commit e fizer push, a VPS esta pronta.

### 5. Agende com cron

Abra o cron:

```bash
crontab -e
```

Adicione esta linha, trocando `/home/ubuntu/script-commit` pelo caminho real do seu clone:

```cron
17 * * * * cd /home/ubuntu/script-commit && MIN_HOURS_BETWEEN_COMMITS=23 HEARTBEAT_FILE=.daily-commit/heartbeat.json SOURCE_TEXT_FILE="meu roteiro.txt" OUTPUT_TEXT_FILE=roteiro.md WORD_STATE_FILE=.daily-commit/word-state.json TARGET_BRANCH=main FORCE_COMMIT=false SKIP_PUSH=false /usr/bin/go run ./scripts/daily_commit.go >> $HOME/daily-commit.log 2>&1
```

Esse cron roda todo minuto `17` de cada hora. O script decide sozinho se ja esta na hora de commitar.

Para ver os logs:

```bash
tail -f ~/daily-commit.log
```

Se o comando `which go` mostrar outro caminho, troque `/usr/bin/go` na linha do cron pelo caminho correto.

## Configuracao

No arquivo `.github/workflows/daily-commit.yml`, ajuste:

- `MIN_HOURS_BETWEEN_COMMITS`: use `23` para commit diario com margem antes de 24h.
- `HEARTBEAT_FILE`: arquivo que sera atualizado pelo commit automatico.
- `SOURCE_TEXT_FILE`: texto-fonte usado para acrescentar uma palavra por commit.
- `OUTPUT_TEXT_FILE`: arquivo gerado com as palavras, por padrao `roteiro.md`.
- `WORD_STATE_FILE`: arquivo que guarda o indice da proxima palavra.
- `cron`: horario/frequencia do watchdog. O valor atual, `17 * * * *`, roda uma vez por hora.
- `go-version`: versao do Go usada pelo GitHub Actions.

## Observacao importante

GitHub Actions agenda workflows com boa confiabilidade, mas nao oferece garantia absoluta de execucao no minuto exato. Rodar de hora em hora e commitar depois de 23h reduz bastante o risco de passar de 24h. Para garantia mais rigida, use o mesmo `scripts/daily_commit.go` em um servidor/runner proprio com cron ou systemd timer.
