# site-sentry-go

Go + SQLite で実装した Web サイト死活監視ツールの MVP です。  
複数 URL の登録、定期チェック、最新状態表示、履歴参照、手動チェック実行ができます。

## 目的
- 個人開発者や小規模チームがローカルで簡単に使える死活監視ツールを提供する
- 過剰な依存を避け、標準ライブラリ中心で保守しやすくする

## 主な機能
- 監視対象（Monitor）の CRUD
- 定期 HTTP GET チェック（timeout 制御あり）
- 最新ステータス（UP/DOWN）、応答時間、最終チェック時刻、失敗理由の保存
- チェック履歴保存と参照
- 手動チェック `POST /monitors/{id}/check`
- ヘルスチェック `GET /healthz`
- 直近24時間成功率（詳細 API/画面）
- ステータスフィルタ（`/monitors?status=UP|DOWN`、`/?status=UP|DOWN`）

---

## アーキテクチャ（MVP）

- `cmd/site-sentry/main.go`: エントリポイント、依存組み立て、graceful shutdown
- `internal/config`: 環境変数読み込み
- `internal/model`: ドメインモデル
- `internal/repository`: SQLite 永続化
- `internal/service`: バリデーション・死活チェック・ユースケース
- `internal/scheduler`: 定期実行ランナー
- `internal/handler`: HTTP API と簡易 HTML
- `internal/db`: スキーマ作成
- `web/templates`: 一覧/詳細の server-rendered HTML

---

## データモデル

### monitors
- id
- name
- url
- interval_seconds
- timeout_seconds
- is_enabled
- last_status
- last_status_code
- last_response_time_ms
- last_checked_at
- consecutive_failures
- last_error_message
- created_at
- updated_at

### check_results
- id
- monitor_id
- status
- status_code
- response_time_ms
- error_message
- checked_at

---

## セットアップ

### 前提
- Go 1.23+

### 起動
```bash
make tidy
make run
```

起動後:
- UI: `http://localhost:8080/`
- healthz: `http://localhost:8080/healthz`

### 環境変数
- `APP_PORT` (default: `8080`)
- `DB_PATH` (default: `./site_sentry.db`)
- `SCHEDULER_TICK_SECONDS` (default: `1`)
- `DEFAULT_RESULT_LIMIT` (default: `20`)
- `REQUEST_TIMEOUT_SECONDS` (default: `10`)
- `SHUTDOWN_TIMEOUT_SECONDS` (default: `10`)

---

## API

### 監視対象作成
```bash
curl -X POST http://localhost:8080/monitors \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"example",
    "url":"https://example.com",
    "interval_seconds":30,
    "timeout_seconds":5,
    "is_enabled":true
  }'
```

### 一覧
```bash
curl http://localhost:8080/monitors
curl 'http://localhost:8080/monitors?status=DOWN'
```

### 詳細
```bash
curl http://localhost:8080/monitors/1
```

### 更新
```bash
curl -X PUT http://localhost:8080/monitors/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"example-updated",
    "url":"https://example.com",
    "interval_seconds":60,
    "timeout_seconds":10,
    "is_enabled":true
  }'
```

### 削除
```bash
curl -X DELETE http://localhost:8080/monitors/1
```

### 手動チェック
```bash
curl -X POST http://localhost:8080/monitors/1/check
```

### 履歴取得
```bash
curl http://localhost:8080/monitors/1/results
```

---

## seed データ投入（任意）

```bash
./scripts/seed.sh
```

---

## テスト

```bash
make test
```

`internal/service/monitor_service_test.go` で最低限のユースケースを検証しています。

---

## Docker

```bash
docker compose up --build
```

- アプリ: `http://localhost:8080`
- DB: `./.data/site_sentry.db` に永続化

---

## 未対応事項 / 今後の拡張

- 認証/認可
- 通知（メール/Slack 等）
- ヒストリのページング・集計強化
- ジョブ優先度や高負荷時の実行制御
- OpenAPI 定義とクライアント生成
- 詳細 UI の改善（グラフ、並び替え等）
