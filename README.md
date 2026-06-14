# Daily Commit Watchdog em Go

Automacao para manter o repositorio privado com commits frequentes, em horarios aleatorios, sem deixar passar 20 horas entre um commit e outro.

O workflow roda a cada 15 minutos, mas o script Go escolhe uma janela aleatoria entre commits. Em dias normais usa `240` a `480` minutos, dando media perto de 4 commits por dia. Em sextas, sabados e feriados usa `120` a `360` minutos, podendo passar de 4 commits. O limite continua sendo 10 commits por dia.

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

Ele usa o arquivo local `meu roteiro.txt` como fonte. A cada execucao valida, o script pega a proxima palavra desse arquivo, acrescenta em `roteiro.md`, atualiza `.daily-commit/word-state.json` e cria um commit. Quando chega no fim do texto, ele volta para a primeira palavra e continua em um novo ciclo.

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

O script nao precisa ficar aberto em loop. O ideal e deixar a VPS ligada 24/7 e chamar o script a cada 15 minutos com `cron`. O script decide sozinho se ja chegou um horario aleatorio de commit.

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
RANDOM_MIN_MINUTES_BETWEEN_COMMITS=240 \
RANDOM_MAX_MINUTES_BETWEEN_COMMITS=480 \
BOOST_RANDOM_MIN_MINUTES_BETWEEN_COMMITS=120 \
BOOST_RANDOM_MAX_MINUTES_BETWEEN_COMMITS=360 \
BOOST_FIXED_DATES=01-01,04-21,05-01,09-07,10-12,11-02,11-15,11-20,12-25 \
BOOST_DATES= \
SAFETY_MAX_MINUTES_WITHOUT_COMMIT=1200 \
MAX_COMMITS_PER_DAY=10 \
COMMIT_DAY_TIMEZONE=America/Sao_Paulo \
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
*/15 * * * * cd /home/ubuntu/script-commit && RANDOM_MIN_MINUTES_BETWEEN_COMMITS=240 RANDOM_MAX_MINUTES_BETWEEN_COMMITS=480 BOOST_RANDOM_MIN_MINUTES_BETWEEN_COMMITS=120 BOOST_RANDOM_MAX_MINUTES_BETWEEN_COMMITS=360 BOOST_FIXED_DATES=01-01,04-21,05-01,09-07,10-12,11-02,11-15,11-20,12-25 BOOST_DATES= SAFETY_MAX_MINUTES_WITHOUT_COMMIT=1200 MAX_COMMITS_PER_DAY=10 COMMIT_DAY_TIMEZONE=America/Sao_Paulo HEARTBEAT_FILE=.daily-commit/heartbeat.json SOURCE_TEXT_FILE="meu roteiro.txt" OUTPUT_TEXT_FILE=roteiro.md WORD_STATE_FILE=.daily-commit/word-state.json TARGET_BRANCH=main FORCE_COMMIT=false SKIP_PUSH=false /usr/bin/go run ./scripts/daily_commit.go >> $HOME/daily-commit.log 2>&1
```

Esse cron roda de 15 em 15 minutos. O script decide sozinho se ja esta na hora aleatoria de commitar.

Para ver os logs:

```bash
tail -f ~/daily-commit.log
```

Se o comando `which go` mostrar outro caminho, troque `/usr/bin/go` na linha do cron pelo caminho correto.

## Configuracao

No arquivo `.github/workflows/daily-commit.yml`, ajuste:

- `RANDOM_MIN_MINUTES_BETWEEN_COMMITS`: menor intervalo aleatorio entre commits. Padrao: `240` minutos.
- `RANDOM_MAX_MINUTES_BETWEEN_COMMITS`: maior intervalo aleatorio entre commits. Padrao: `480` minutos.
- `BOOST_RANDOM_MIN_MINUTES_BETWEEN_COMMITS`: menor intervalo em sextas, sabados e feriados. Padrao: `120` minutos.
- `BOOST_RANDOM_MAX_MINUTES_BETWEEN_COMMITS`: maior intervalo em sextas, sabados e feriados. Padrao: `360` minutos.
- `BOOST_FIXED_DATES`: feriados fixos no formato `MM-DD`, separados por virgula.
- `BOOST_DATES`: feriados moveis no formato `YYYY-MM-DD`, separados por virgula.
- `SAFETY_MAX_MINUTES_WITHOUT_COMMIT`: limite de seguranca sem commit. Padrao: `1200` minutos, ou 20h.
- `MAX_COMMITS_PER_DAY`: limite diario de commits do roteiro. Padrao: `10`.
- `COMMIT_DAY_TIMEZONE`: fuso usado para contar o limite diario. Padrao: `America/Sao_Paulo`.
- `HEARTBEAT_FILE`: arquivo que sera atualizado pelo commit automatico.
- `SOURCE_TEXT_FILE`: texto-fonte usado para acrescentar uma palavra por commit.
- `OUTPUT_TEXT_FILE`: arquivo gerado com as palavras, por padrao `roteiro.md`.
- `WORD_STATE_FILE`: arquivo que guarda o indice da proxima palavra.
- `cron`: horario/frequencia do watchdog. O valor atual, `*/15 * * * *`, roda a cada 15 minutos.
- `go-version`: versao do Go usada pelo GitHub Actions.

## Observacao importante

GitHub Actions agenda workflows com boa confiabilidade, mas nao oferece garantia absoluta de execucao no minuto exato. Para garantia mais rigida de nao passar 20h, use o mesmo `scripts/daily_commit.go` em uma VPS/runner proprio com cron a cada 15 minutos.
