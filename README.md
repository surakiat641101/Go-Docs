# Go-Docs
// ===== แปลง HTML + CSS เป็นเอกสาร .docx ===== //

API ด้วย [Go Fiber](https://gofiber.io) สำหรับแปลง HTML + CSS เป็นไฟล์ Word (.docx)
เขียน OOXML เองด้วย `archive/zip` จึงไม่ต้องใช้ไลบรารีที่มีค่าลิขสิทธิ์ (เช่น UniOffice) และรองรับภาษาไทย
(ตั้งค่าฟอนต์/ขนาดแบบ complex script `w:cs` / `szCs` / `bCs` ให้ Word แสดงผลภาษาไทยถูกต้อง)

## เริ่มใช้งาน

```bash
go run .            # http://localhost:3000  (เปลี่ยนพอร์ตด้วย PORT=8080)
go test ./...
```

เปิด `http://localhost:3000` จะเจอหน้า playground สำหรับแก้ HTML ดูตัวอย่าง และดาวน์โหลดไฟล์ .docx

### ตั้งค่าด้วย `.env`

```bash
cp .env.example .env
```

| ตัวแปร                | ค่าเริ่มต้น | ความหมาย                                                      |
|-----------------------|-----------|---------------------------------------------------------------|
| `PORT`                | 3000      | พอร์ตที่ server ฟัง (รวมถึงในคอนเทนเนอร์)                         |
| `HOST_PORT`           | 3000      | พอร์ตฝั่งเครื่อง host ที่ docker-compose เปิดให้เข้าถึง                |
| `BODY_LIMIT_MB`       | 20        | ขนาด request สูงสุด (MB)                                        |
| `ALLOW_REMOTE_IMAGES` | false     | อนุญาตให้ server ดาวน์โหลดรูปจาก URL (ระวังเรื่อง SSRF)               |
| `CORS_ALLOW_ORIGINS`  | `*`       | origin ที่อนุญาต คั่นด้วย comma                                    |

ค่าที่ตั้งเป็น environment variable จริงจะมีผลเหนือ `.env` เสมอ ส่วนไฟล์ `.env` ไม่ถูก commit (อยู่ใน `.gitignore`)

### Docker

```bash
docker compose up -d --build     # build แล้วรันที่ http://localhost:${HOST_PORT}
docker compose logs -f
docker compose down
```

image เป็นแบบ multi-stage (build ด้วย `golang:1.26-alpine` แล้วรันบน `alpine` ด้วย user ที่ไม่ใช่ root)
มี `HEALTHCHECK` ยิงไปที่ `/health` และ compose รันแบบ `read_only` ด้วยตัวเลือก `no-new-privileges`

## API: `POST /api/convert`

ส่ง HTML มาได้ 3 แบบ แล้วจะได้ไฟล์ `.docx` กลับไป (header `Content-Disposition` รองรับชื่อไฟล์ภาษาไทย)

**JSON**
```bash
curl -X POST http://localhost:3000/api/convert \
  -H "Content-Type: application/json" \
  -d '{"html":"<h1>สวัสดี</h1><p>เอกสาร</p>","filename":"รายงาน","options":{"font":"TH Sarabun New","fontSize":16,"pageSize":"A4","landscape":false,"marginMM":25.4}}' \
  -o report.docx
```

**HTML ตรง ๆ** (ส่ง options เป็น query string)
```bash
curl -X POST "http://localhost:3000/api/convert?filename=report&font=TH%20Sarabun%20New&fontSize=16" \
  -H "Content-Type: text/html; charset=utf-8" --data-binary @page.html -o report.docx
```

**อัปโหลดไฟล์** (multipart: `file` หรือ `html`, `filename`, `font`, `fontSize`, `pageSize`, `landscape`)
```bash
curl -X POST http://localhost:3000/api/convert -F "file=@page.html" -o page.docx
```

## API: `POST /api/convert/html-css` (ส่ง HTML และ CSS แยกกัน)

เหมาะกับกรณีที่เก็บ template HTML กับไฟล์ CSS ไว้แยกกัน
CSS ที่ส่งมาจะถูกนำไปใช้**หลัง** `<style>` ที่อยู่ใน HTML จึงชนะเมื่อ specificity เท่ากัน
(แต่ `style="..."` ใน HTML ยังมีผลเหนือกว่า)

**JSON**
```bash
curl -X POST http://localhost:3000/api/convert/html-css \
  -H "Content-Type: application/json" \
  -d '{"html":"<h1 class=\"title\">สวัสดี</h1>","css":".title{color:#1F4E79;text-align:center}","filename":"รายงาน","options":{"font":"TH Sarabun New","fontSize":16}}' \
  -o report.docx
```

**อัปโหลดไฟล์** (multipart: `htmlFile` + `cssFile` หรือส่งเป็นข้อความในฟิลด์ `html` + `css` ถ้าส่งทั้งไฟล์และข้อความ จะใช้ไฟล์)
```bash
curl -X POST http://localhost:3000/api/convert/html-css \
  -F "htmlFile=@template.html" -F "cssFile=@style.css" -F "filename=รายงาน" -F "fontSize=16" \
  -o report.docx
```

endpoint นี้รับเฉพาะ `application/json` และ `multipart/form-data` (ส่งแบบอื่นจะได้ 415)

| option      | ค่าเริ่มต้น | หมายเหตุ                           |
|-------------|-----------|------------------------------------|
| `font`      | Tahoma    | ฟอนต์หลักของเอกสาร                 |
| `fontSize`  | 11        | หน่วย pt                           |
| `pageSize`  | A4        | `A4` หรือ `Letter`                  |
| `landscape` | false     | แนวนอน                             |
| `marginMM`  | 25.4      | ระยะขอบกระดาษ (มม.)                |
| `title`     | `<title>` | ชื่อเอกสารใน properties ของไฟล์      |
| `author`    | –         |                                    |

## HTML / CSS ที่รองรับ

- **แท็ก:** `h1`–`h6` (ใช้สไตล์ Heading ของ Word ทำให้สร้างสารบัญได้), `p`, `div` และแท็ก block อื่น ๆ, `br`, `hr`,
  `b/strong`, `i/em`, `u`, `s/del`, `sub`, `sup`, `mark`, `small`, `code`, `pre`, `blockquote`, `a` (ลิงก์ที่คลิกได้),
  `ul`/`ol` (ซ้อนได้ 9 ระดับ รองรับ `start` และ `type`), `table` (`thead` ซ้ำหัวตารางทุกหน้า, `colspan`, `rowspan`, ตารางซ้อน, `caption`),
  `img` (data URI แบบ base64: PNG/JPEG/GIF), `dl/dt/dd`, `font`
- **CSS:** `<style>` และ `style="..."` โดยรองรับ selector แบบ tag, `.class`, `#id`, descendant (`table td`), child (`>`)
  และเรียงตาม specificity และ `!important` (rule ที่มี pseudo-class เช่น `:hover` จะถูกข้าม)
- **Properties:** `color`, `background(-color)`, `font-family`, `font-size` (pt/px/em/rem/%/keyword), `font-weight`, `font-style`,
  `text-decoration`, `text-align`, `text-indent`, `line-height`, `margin(-top/-bottom/-left)`, `vertical-align`, `white-space`,
  `width`/`height` (รูปและตาราง), `border` (ตาราง), `display: none|block|inline`, `list-style-type`
  (รวม `thai` → ๑ ๒ ๓), `page-break-before/after` / `break-before/after`
- **ไม่รองรับ:** flex/grid, float, position, รูปแบบ SVG/WebP และสไตล์ border ราย cell

> รูปจาก URL ภายนอก (`http(s)://`) ถูกปิดไว้เป็นค่าเริ่มต้นเพื่อป้องกัน SSRF ถ้าต้องการให้ดึงรูปได้ ให้ตั้งค่า `ALLOW_REMOTE_IMAGES=true`

## โครงสร้าง

```
main.go                                 โหลด config, ประกอบ dependency (service → controller → router) แล้วสตาร์ต server
web/index.html, web/web.go              หน้า playground (embed ลงใน binary)
internal/config/config.go               โหลดค่าจาก .env / environment
internal/router/router.go               สร้าง Fiber app: middleware + route ทั้งหมด
internal/controller/document_controller.go  รับ request (JSON / multipart / raw) → เรียก service → ตอบกลับ
internal/service/document_service.go    business logic: ตรวจ input, ตั้งค่าฝั่ง server แล้วสร้าง .docx (ไม่ผูกกับ Fiber)
internal/helper/request.go              ตรวจ Content-Type, อ่านไฟล์ที่อัปโหลด, อ่าน options จาก form/query
internal/helper/response.go             ส่งไฟล์ .docx + Content-Disposition รองรับชื่อไฟล์ภาษาไทย
internal/htmldocx/convert.go  เดิน HTML tree แล้วสร้าง paragraph/run/table/list/image
internal/htmldocx/css.go      parser ของ CSS, selector, หน่วย และสี
internal/htmldocx/style.go    computed style + inheritance
internal/htmldocx/package.go  ประกอบไฟล์ .docx (zip, rels, styles, numbering)
```
