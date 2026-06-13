# kbkb

**Puyo Puyo on Kubernetes.** Nodes are columns, Pods are puyos — connect four
same-colored Pods and they get deleted. Chain reactions included.

![play](docs/play.gif)

You place falling pairs of Pods from a web UI. Under the hood every move is
real Kubernetes: a custom scheduler binds the Pods to Nodes, a controller
deletes connected groups, and the Pod Events panel shows each
`Running → Terminating → deleted` transition as it happens.

## Install

Requirements: a Kubernetes cluster and
[cert-manager](https://cert-manager.io/docs/installation/).

```bash
kubectl apply -f https://raw.githubusercontent.com/omakeno/kbkb/master/install/kbkb.yaml
```

This installs the CRD, the controllers and the scheduler into the
`kbkb-system` namespace.

To uninstall:

```bash
kubectl delete -f https://raw.githubusercontent.com/omakeno/kbkb/master/install/kbkb.yaml
```

## Play

```bash
# start a game in the default namespace
kubectl apply -f https://raw.githubusercontent.com/omakeno/kbkb/master/config/samples/kbkb.yaml

# open the web UI
kubectl -n kbkb-system port-forward svc/kbkb-scheduler-ui 8765:8765
```

Open http://localhost:8765 and play:

| Key | Action |
|---|---|
| ← → | move |
| Z / X | rotate left / right |
| ↓ | soft drop (press again while grounded to lock) |
| ↑ / Space | hard drop |

New pairs spawn automatically once every Pod on the field is Ready. The game
ends when a column reaches the height limit; clean up the namespace
(`kubectl delete pods --all`) to start over. The **mode** button switches to
auto-play.

Watch the field from a terminal, too:

```bash
go install github.com/omakeno/kbkb/v2/cmd/kubectl-kbkb@latest
kubectl kbkb -w        # live ASCII view of the field
```

## Configuration

A game is configured by a `Kbkb` resource in the namespace you play in:

```yaml
apiVersion: k8s.omakenoyouna.net/v1beta1
kind: Kbkb
metadata:
  name: kbkb
spec:
  kokeshi: 4                 # pods that must connect to erase (try 2 for chaos)
  excludeControlPlane: true  # don't use control-plane nodes as columns
  spawn:
    enabled: true
    maxHeight: 12            # column height limit; reaching it is game over
    disableGameOver: true    # endless mode
  versus:
    opponentNamespace: player2
    garbageRate: 2           # send (erased ÷ 2) garbage pods to the opponent
```

All fields except `kokeshi` are optional. Pod colors come from the
`kbkb.k8s.omakenoyouna.net/color` annotation (red / green / yellow / blue /
purple) — spawned Pods get a random color from a mutating webhook, and any
Pod without a color is white: it never erases and blocks chains.

`kubectl get kbkb` shows your score:

```
NAME   KOKESHI   PHASE   CHAIN   MAXCHAIN
kbkb   4         Idle    0       7
```

The goal is a 19-chain. Good luck.

## Versus mode

Two namespaces, two players. Every chain you make rains unerasable white
"garbage" Pods onto your opponent's field.

```bash
kubectl apply -f https://raw.githubusercontent.com/omakeno/kbkb/master/config/samples/versus.yaml
```

Run one scheduler per player (`--namespace=player1` / `--namespace=player2`)
and open one browser tab each.

## Metrics

The manager exposes Prometheus metrics on `:8080/metrics`, including
`kbkb_chain_current`, `kbkb_max_chain`, `kbkb_erased_pods_total`,
`kbkb_all_clear_total` and `kbkb_ojama_sent_total` — chains look great on a
Grafana dashboard.

## Development

```bash
make            # generate, lint, test, build everything
make deploy     # build images and deploy your working tree (kind etc.)
make installer  # regenerate install/kbkb.yaml
```

Binaries live under `cmd/`, the game logic under `pkg/field`, and the
controllers/webhook/scheduler under `internal/`. The demo GIF is reproducible
with `hack/record/`.

## License

Apache-2.0
