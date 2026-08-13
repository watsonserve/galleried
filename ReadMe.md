## 設計

### 每張圖都有對應3個質量級別的圖片文件：原圖、預覽圖、縮略圖

  - 3張圖使用同一個*版本Id*作為`etag（uuid）`
  - 每張圖有自己的`hash（sha256）`
  - 3張圖使用同一文件名，但不一定同一種格式，所以可能使用不同的擴展名：
    * 原圖使用原擴展名，可以是：`png` `jpg` `heic` `cr2` `webp`
    * 預覽圖和縮略圖使用`webp`
  - 底層存儲上，不同質量級別的圖片被存放在各自質量級別的目錄下

### 圖片列表

列表使用`Array<ResUserImg>`類型

```go
type ResUserImg struct {
    Filename string
    ETag     string
    CTime    int64
}
```

  - 客戶端可以存儲該列表到本地，但客戶端需要負責檢查該列表是否需要同步
  - 客戶端請直接使用對外路徑：
    * 縮略圖=`/thumb/{文件名}`
    * 預覽圖=`/preview/{文件名}`
    * 原圖=`/{文件名}`

## Picture API（對應 action/picture.go）

** 需要登入（Session Cookie），否則 `401` **

預設 `path_prefix` 為 `/`（`galleried.conf` 留空時）。

## 客戶端調用建議

### 1) 目錄與快取拆分

- 把「圖片列表」當成元資料索引，不要把它當成圖片本體
- 客戶端至少維護兩層快取：
  - **列表快取**：`filename -> {etag, ctime}`
  - **圖片快取**：`{filename, lev, etag} -> blob`
- 同一張圖的 `raw/preview/thumb` 共用同一個版本 `etag`；只要 `etag` 變了，三個級別都要一起失效

### 2) 列表同步流程

1. 先抓 `GET /`
2. 將回傳的 `Array<ResUserImg>` 和本地列表比對
3. 規則：
   - `filename` 不存在於本地：新增
   - `filename` 存在但 `etag` 不同：視為同一張圖已更新，清掉這張圖的所有圖片快取，再重新拉取需要的級別
   - `filename` 在本地存在但服務端列表沒有：視為已刪除，清掉本地元資料與圖片快取
4. `ctime` 可作為排序/顯示用，但同步判定以 `etag` 為準

### 3) 圖片快取策略

- 下載圖片前先看本地是否已有相同 `{filename, lev, etag}`
- 若已存在，優先直接使用本地快取
- 若要重新驗證，可對圖片請求帶 `If-None-Match`，命中時回 `304`
- 一旦列表同步發現 `etag` 變更，應立刻讓該圖的 `raw/preview/thumb` 全部失效

### 4) CRUD 對應

- **Create**：`PUT /{filename}`，只接受 `raw`
- **Read**：`GET /{filename}`、`GET /preview/{filename}`、`GET /thumb/{filename}`；可搭配 `HEAD` 與 `If-None-Match`
- **Update**：同樣使用 `PUT /{filename}`，但必須帶舊的 `If-Match`
- **Move**：`MOVE /{filename}`，透過 `Destination` 在正常區與回收站之間切換
- **Delete**：`DELETE /{filename}`（從回收站永久刪除）

### 1) 列表

`GET /`

- 回傳 JSON（`StdJSONResp`），`data` 為 `Array<ResUserImg>`
- 可帶 `Range` header（僅支援單一 segment；多段會回錯）

### 2) 讀取圖片（僅經 nginx）

`GET /thumb/{filename}`
`GET /preview/{filename}`
`GET /{filename}`（原圖）

> nginx 會改寫轉發成 `/{filename}?lev=thumb|preview`；原圖等效 `lev=raw`。

- `lev` 不帶時預設為 `raw`
- `lev` 非 `raw/preview/thumb` 時回 `404`
- 支援 `If-None-Match`（強比較）；命中回 `304`
- 成功時回應 header：
  - `Vary: Cookie`
  - `Content-Type`
  - `Content-Length`
  - `Content-Digest: sha-256=:...:`
  - `ETag: "..."`（強 ETag）

### 3) 上傳（PUT）

`PUT /{filename}`

- 不可以上傳縮略圖或者預覽圖，否則`405`
- `Content-Type` 必須是 `image/*`，否則 `415`
- 必須帶 `Content-Digest` 且含 `sha-256`，否則 `400`
- 必須帶 `If-Match` 且需為強 ETag（`W/"..."` 不接受），否則 `412`
- `If-Match` 與現況不符回 `412`；檔案不存在（或已移除）回 `410`
- 可帶 `Content-Encoding: gzip`；若是其他 encoding 會 `400`
- 寫入檔案失敗回 `503`；寫入索引失敗回 `400`
- 成功回 `201`，並設定：
  - `ETag: "..."`（新版本）
  - `Location: {origin}/{filename}` 伺服器會用 `Origin`（或 `Referer` / `Host`）組出 `Location`

### 4) 生成預覽圖（POST，僅 raw）

`POST /{filename}` 或 `POST /{filename}?lev=raw`

- 需要登入，否則 `401`
- `lev=preview/thumb` 時，`POST` 會回 `405`
- 成功回 `201`
- 找不到原圖或生成失敗回 `404`

### 5) 刪除圖片（DELETE）

`DELETE /{filename}`

- 需要登入，否則 `401`
- 必須帶 `If-Match`，否則 `412`
- 只能從回收站刪除（`rtime>0`）：成功回 `204`
- `If-Match` 不符回 `412`
- 當前既不在正常列表也不在回收站時回 `410`
- 回收站刪除時，會在單一 PostgreSQL SQL 中完成「刪除回收站記錄 + 檢查 `etag` 引用數」；只有「無任何引用」才會刪除實體檔案與 `res_thumb` 記錄
- 建議客戶端在本地同時移除該圖片的列表快取與 `raw/preview/thumb` 快取

### 6) 移動圖片（MOVE）

`MOVE /{filename}`

- 需要登入，否則 `401`
- 需要帶 `Destination`
- `Destination` 的檔名必須與來源相同
- 來源 `/{filename}` 且目標 `/recycle/{filename}`，表示移入回收站
- 來源 `/recycle/{filename}` 且目標 `/{filename}`，表示從回收站恢復
- 成功回 `204`

## 更新一張圖片示例

```http
PUT /foo.cr2 HTTP/1.1
Content-Type: image/cr2
Origin: https://store.watsonserve.com
Content-Digest: sha-256=:abcdefg...:
If-Match: "uuid1234..."
Expect: 100-continue
Content-Length: 1000000
Cookie: abc=def
```

## configure
```
# pg_db
db_user=foo
db_passwd=bar
db_host=127.0.0.2
db_name=galleried_db
db_port=5432

#redis
redis_address=127.0.0.3
redis_password=

#session & cookie
sess_name=sess
cookie_prefix=galleried
session_prefix=galleried
domain=localhost

# files store
root=/home/you/pictures

# server
# path_prefix 留空時預設為 /
# path_prefix=
#listen=127.0.0.1:80
listen=:80
```