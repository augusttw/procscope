# procscope

`procscope` é uma CLI de observabilidade local para Linux. Ela mostra, em segundos, o que um processo está consumindo e com quais serviços ele conversa, sem agentes ou uma stack externa.

O projeto usa somente a biblioteca padrão do Go e `/proc`. A coleta fica atrás da interface `observe.Source`, permitindo adicionar Netlink, eBPF, pprof e OpenTelemetry sem acoplar essas tecnologias à CLI ou às análises.

## Instalação

Requer Go 1.22+ e Linux.

```sh
go install github.com/augusttw/procscope/cmd/procscope@latest
# ou, no repositório
make build
```

## Uso rápido

```sh
# Executa e acompanha um comando até ele terminar
procscope run -- ./meu-servidor --port 8080

# Acompanha um processo; Ctrl-C encerra apenas o procscope
procscope attach 1234
procscope attach --once 1234
procscope attach --once --json 1234

# Rede pertencente ao processo
procscope ports 1234
procscope connections 1234

# Trace JSON de 30 segundos e diagnóstico
procscope record --duration 30s --interval 500ms --output before.json 1234
procscope doctor --file before.json

# Compare snapshots ou a última amostra de dois traces
procscope diff before.json after.json
```

Dependências são inferidas pelas portas remotas mais comuns: PostgreSQL (`5432`), Redis (`6379`) e HTTP/HTTPS (`80`, `443`, `3000`, `8000`, `8080`, `8443`). Isso é uma indicação, não inspeção de protocolo.

## Métricas e diagnóstico

Cada amostra contém CPU desde a amostra anterior, RSS, memória virtual, uptime, threads, descritores, bytes de I/O e sockets TCP/TCP6. A primeira leitura de CPU é zero porque ainda não existe uma amostra anterior.

`doctor` aplica regras transparentes para CPU elevada, estados zombie/D, muitos descritores, crescimento de RSS/FDs e excesso de `TIME_WAIT`. Em um snapshot isolado ele avalia apenas o estado atual; um trace permite detectar tendências.

## Arquitetura

```text
cmd/procscope       ponto de entrada
internal/cli        comandos e experiência de terminal
internal/observe    contrato Source e amostrador
internal/procfs     coletor Linux atual
internal/model      snapshots/traces versionados
internal/analysis   diff e regras do doctor
internal/storage    persistência JSON
internal/format     apresentação textual
```

Limitações atuais: apenas TCP/TCP6; permissões do kernel podem ocultar FDs de outros usuários; a associação de dependência é baseada em porta; `USER_HZ=100` é assumido, como nos kernels Linux suportados pelas arquiteturas usuais.

## Desenvolvimento

```sh
make test
make vet
make build
```
