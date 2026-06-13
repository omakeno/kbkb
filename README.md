# くべくべ (kbkb) v2

Kubernetes 上でぷよぷよを再現するカオスエンジニアリング(?)ツール。
Node を列、Pod をぷよとして、同じ色の Pod が 4 個隣接すると自動削除されます。

![play](docs/play.gif)

WebUI からペアを移動・回転・ドロップして配置(=カスタムスケジューラーが Pod を Node にバインド)。
4 個つながると消去コントローラーが Pod を削除し、右下の Pod Events に
`Running → Terminating → deleted` の遷移が流れます。上の GIF では 2 連鎖が発生しています。

旧 3 リポジトリ([kbkb](https://github.com/omakeno/kbkb) /
[kbkb-controller](https://github.com/omakeno/kbkb-controller) /
[kubectl-kbkb](https://github.com/omakeno/kubectl-kbkb))を
単一モノレポにリファクタリングし、バックログだった「ゲームループ」一式
(Pod 生成・ランダム色付け・2 個ずつ操作するスケジューラー)を実装したものです。
目標は **19 連鎖**。

## アーキテクチャ

```
                 ┌──────────────────────────────────────────────┐
                 │ kbkb-manager                                  │
  Kbkb CR ──────▶│  ├ 消去コントローラー: 4個隣接で削除・連鎖計測 │
                 │  ├ Spawnコントローラー: 安定したらペアを生成   │
                 │  └ Webhook: 生成Podにランダムに色を付与        │
                 └──────────────────────────────────────────────┘
                          │ spawn (schedulerName: kbkb-scheduler)
                          ▼
                 ┌──────────────────────────────────────────────┐
   ブラウザ ◀───▶│ kbkb-scheduler (ゲームサーバー)               │
   ←→↑Space      │  ├ WebUI: 移動・回転・ドロップ / SSEで状態配信 │
                 │  └ ペアを2個ずつ Binding APIでNode(列)へ配置  │
                 └──────────────────────────────────────────────┘
```

ゲームループ:
**Spawn**(2個生成)→ **Webhook**(ランダム色付け)→ **Scheduler**(プレイヤーが列に配置)
→ Running/Ready → 4個隣接で**消去** → 再安定後また消えれば**連鎖++** → 次のSpawn → …
→ 列が `maxHeight` を超えたら **GameOver**。

## 構成

| パス | 内容 |
|---|---|
| `pkg/field` | コアロジック: フィールド構築・隣接グループ判定(反復DFS) |
| `pkg/printer` | ターミナル描画(ANSI上書き出力を内蔵) |
| `api/v1beta1` | `Kbkb` CRD |
| `internal/controller` | 消去コントローラー / Spawnコントローラー |
| `internal/webhook` | Pod色付け Mutating Webhook |
| `internal/scheduler` | カスタムスケジューラー + WebUI(`go:embed`) |
| `internal/cli` | kubectl プラグイン |
| `cmd/{manager,scheduler,kubectl-kbkb}` | 各バイナリ |
| `config/` | CRD / RBAC / Deployment / cert-manager(kustomize) |

## Kbkb CRD

```yaml
apiVersion: k8s.omakenoyouna.net/v1beta1
kind: Kbkb
metadata:
  name: kbkb-sample
spec:
  kokeshi: 4              # 何個隣接で消すか(2個消し・6個消しも可)
  excludeControlPlane: true  # control-planeノードを列から除外(任意)
  nodeSelector:           # ラベルが一致するノードだけを列にする(任意)
    kbkb: "true"
  spawn:
    enabled: true         # 全PodがReadyになったらPodを生成
    pair: 2               # 一度に生成する数
    image: registry.k8s.io/pause:3.10
    schedulerName: kbkb-scheduler
    maxHeight: 12         # 列の高さ制限。超えたらGameOver
    disableGameOver: true # エンドレスモード: 高さ制限でもゲームを止めない(任意)
  versus:                 # 対戦モード(任意)
    opponentNamespace: player2
    garbageRate: 2        # 消した数÷この値のおじゃまPodを相手に送る
status:
  phase: Idle             # Idle / Erasing / GameOver
  chain: 0                # 進行中の連鎖数
  maxChain: 7             # 最高連鎖(目標19)
  totalErased: 84
  allClears: 1            # 全消し回数
```

色は Pod のアノテーション `kbkb.k8s.omakenoyouna.net/color`
(red / green / yellow / blue / purple)。色なし・white は消えず、巻き込まれません。
おじゃまPod(白)はラベル `kbkb.k8s.omakenoyouna.net/ojama=true` 付きで生成され、
スケジューラーがペアを経由せず即落下させます。

## 遊び方

```bash
# 1. CRDと一式をデプロイ(cert-manager が必要)
make install
make docker-build deploy   # kind なら: kind load docker-image kbkb-manager:latest kbkb-scheduler:latest

# 2. ゲーム開始
kubectl apply -f config/samples/kbkb.yaml

# 3. WebUIを開いて操作
kubectl -n kbkb-system port-forward svc/kbkb-scheduler-ui 8765:8765
# → http://localhost:8765  (←→: 移動, ↑/X: 回転, Space: ドロップ)
```

ローカル実行(クラスタ外)も可能です:

```bash
make run-manager     # Webhook無効で起動(色は手動アノテーション)
make run-scheduler   # http://localhost:8765 でUI
```

### kubectl プラグイン

```bash
go build -o ~/bin/kubectl-kbkb ./cmd/kubectl-kbkb
kubectl kbkb            # 現在のnamespaceを表示
kubectl kbkb -w         # watch
kubectl kbkb -L         # 大きい表示
kubectl kbkb --demo     # labelハッシュで色付け(既存クラスタのデモ用)
kubectl kbkb --exclude-control-plane  # control-plane列を非表示
```

### 対戦モード

```bash
kubectl apply -f config/samples/versus.yaml
# プレイヤーごとにスケジューラーを起動
go run ./cmd/scheduler --namespace=player1 --listen=:8765
go run ./cmd/scheduler --namespace=player2 --listen=:8766
```

連鎖するたびに `消した数 ÷ garbageRate` 個の白おじゃまPodが相手のフィールドに降ります。

## メトリクス

manager の `/metrics`(:8080)で公開:

- `kbkb_chain_current` / `kbkb_max_chain` — 連鎖数(Grafanaで19連鎖を観測しましょう)
- `kbkb_erased_pods_total` / `kbkb_spawned_pods_total`
- `kbkb_all_clear_total` / `kbkb_ojama_sent_total` / `kbkb_game_over`

## v1 からの主な変更

- 3リポジトリ + 共通ライブラリ → 単一モジュール `github.com/omakeno/kbkb/v2`
- Go 1.13 / controller-runtime v0.6 → Go 1.26 / controller-runtime v0.24
- 隣接判定: 再帰DFS + O(n²)スライス探索 → visited配列 + 反復DFS
- バグ修正: 未スケジュールPodが先頭Nodeの列に積まれていた / Terminating中のPodが安定扱いだった
- `bashoverwriter` 依存 → 数行のANSI実装(`printer.Overwriter`)
- kubectl プラグインは `cli-runtime` 採用(`-n`/`--context`等が標準動作)
- Pod の積み順: 作成順 → スケジューラーが付与する `drop-order` アノテーション順
  (回転操作で上下を入れ替えられるようにするため)
- バックログ実装: Spawnコントローラー / 色付けWebhook / 2個ずつ操作するスケジューラー
- 追加: WebUI操作・連鎖/全消し/GameOver判定・対戦モード・Prometheusメトリクス

## 開発

```bash
make            # generate + manifests + fmt + vet + test + build
make test
make manifests  # controller-gen で CRD/RBAC/Webhook を再生成
```

デモGIF(`docs/play.gif`)は `hack/record/` で再生成できます
(headless Chrome をDockerで起動し、キー操作をスクリプトしてGo標準ライブラリでGIF化):

```bash
docker run -d --rm --name kbkb-cdp -p 9222:9222 \
  -v $PWD/hack/record/fonts:/usr/share/fonts/noto:ro chromedp/headless-shell
cd hack/record && go run .   # 別シェルで ./stage-chain.sh を流すと連鎖シーンが入る
```
