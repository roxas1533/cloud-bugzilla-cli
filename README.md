# bugzilla-cli

Bugzilla REST API を操作する CLI ツール。

## セットアップ

### 開発環境

```bash
nix develop
```

### 設定ファイル

`~/.config/bugzilla-cli/config.toml` を作成する。

```toml
base_url = "http://your-bugzilla.example.com/bugzilla/rest"
browse_url = "http://your-bugzilla.example.com/bugzilla/show_bug.cgi?id=%d"

[drive_map]
W = '\\192.168.x.x\Share'

[fix_template]
fixed_fields = """
【ポート開放条件の変更内容】
なし

【変更した設定ファイル】
なし

【アップデート時に必要な手順】
なし

【注意点】
なし
"""

[verify_template]
result = "OK"
```

### 環境変数

`.env` で管理し、direnv (`use flake` + `dotenv`) で自動読み込み。

| 変数 | 必須 | 説明 |
|---|---|---|
| `BUGZILLA_API_KEY` | Yes | Bugzilla API キー (Preferences > API Keys で生成) |
| `BUGZILLA_USER` | No | fix 時に Assignee を自分に変更する場合のメールアドレス |

## コマンド

### show

バグの詳細とコメントを表示する。

```bash
bugzilla show <bug_id>
bugzilla show --md <bug_id>   # Markdown 形式で出力
```

### fix

バグを RESOLVED/FIXED に変更し、テンプレートコメントを付与する。

```bash
# 対話モード
bugzilla fix <bug_id>

# フラグモード (CI/AI Agent 用)
bugzilla fix <bug_id> \
  --version "v1.0.0" \
  --summary "修正概要" \
  --impact "影響する機能" \
  --testfile "検証項目表パス" \
  --cause "原因"       # オプション
  --spread "水平展開先" # オプション (空なら「なし」)
```

### verify

バグを VERIFIED に変更し、確認コメントを付与する。

```bash
# 対話モード
bugzilla verify <bug_id>

# フラグモード
bugzilla verify <bug_id> \
  --env "確認環境" \
  --testfile "検証項目表パス"
```

## ドライブパス変換

`config.toml` の `[drive_map]` に設定したドライブレターは、`--testfile` 指定時に自動でネットワークパスに変換される。

```
W:\foo\bar.xlsx → \\192.168.x.x\Share\foo\bar.xlsx
```
