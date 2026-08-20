# PRO X2 SUPERSTRIKE macOS CLI 專案提案

## 專案目標

在 macOS 上打造一個原生 CLI 工具,透過 HID++ 2.0 協定讀取(第一階段)、最終寫入(第二階段)Logitech
PRO X2 SUPERSTRIKE 的裝置參數,涵蓋 DPI、輪詢率(polling rate)、按鍵映射、電量,以及該機種特有的
Haptic Inductive Trigger System(HITS,含 actuation point、rapid trigger reset)。

不依賴 Logitech 官方軟體(G HUB / Logi Options+),完全走 hidraw/IOKit 底層通訊。

## 背景與已知資訊

- Logitech 沒有公開 API 或官方 CLI。G HUB 在 macOS 上只有 GUI,協定為私有 HID++。
- HID++ 2.0 的**標準**功能集(root feature table、DPI、按鍵重映射、電量)已被社群完整逆向,
  記錄在 `libratbag` 專案中,可直接作為協定字典使用。
- PRO X2 SUPERSTRIKE 是 2026 年 2 月才上市的新機種,其**私有**功能(HITS 力度曲線/觸發點)
  尚未被主流工具(Solaar、libratbag)收錄。
- 已找到一個**直接針對本機種**的社群逆向專案:`mclol0/linux-superstrike`,實作了 DPI、
  polling rate、haptics、按鍵重映射、onboard profile 讀寫,並附有 `REVERSE_ENGINEERING.md`
  說明逆向過程與 payload 格式。雖然是 Linux native app,但 **HID++ report 格式與平台無關**,
  可直接作為協定參考,只需重新實作 macOS 端的 I/O 層。

## 參考資源(必讀,依重要性排序)

1. **https://github.com/mclol0/linux-superstrike**
   本機種專屬逆向成果。重點看:
   - `REVERSE_ENGINEERING.md` — HITS 與 onboard profile 的 payload 格式
   - `-probe` / `-scan` 指令的原始碼實作,理解他們怎麼列 feature table
2. **https://github.com/libratbag/libratbag**
   HID++ 2.0 標準功能的 reference 實作。
   - `src/hidpp20.h` / `src/hidpp20.c` — feature ID enum、command/response 格式
   - Wiki 的 `Adding-a-new-device` 頁面 — 逐步除錯流程範例
3. **https://github.com/libratbag/ratbag-toolbox**
   `hidpp-dissector.lua`(Wireshark HID++ 封包解析外掛)+ 多款滑鼠的 init sequence
   pcapng 範例封包,除錯階段拿來對照格式。
4. **https://github.com/cvuchener/hidpp**
   C++ 寫的 HID++ 工具集與 library,`hidpp-list-features` 等 CLI 工具可用來快速驗證
   protocol 假設,不一定要整合進最終專案,但很適合當驗證工具用。
5. Nestor Lopez Casado 的 HID++ 2.0 官方文件(Logitech 前員工釋出的草案文件),
   `libratbag` 原始碼註解裡有連結,是所有社群逆向的原始依據。

## 技術棧與架構決策

- **語言:Go**(對齊使用者現有技術棧,並產出單一 binary,無需 runtime 依賴)
- **HID 存取:`github.com/sstallion/go-hid`**(hidapi 的 Go binding,跨平台)
- **封包格式:HID++ 2.0 long report**
  - Report ID `0x11`,20 bytes
  - Byte 1 = device index(LIGHTSPEED dongle 需正確處理多裝置 index,直連或藍牙通常為 `0xff`)
  - Byte 2 = feature index(需先查 root feature table `0x0000` 取得)
  - Byte 3 高 4 bits = function ID,低 4 bits = software ID(自訂,方便對應 response)
- **macOS 特殊需求:**
  - 需要 Input Monitoring / Accessibility 權限才能存取 raw HID report
  - LIGHTSPEED 接收器會在系統上列出多個 HID interface,需先 enumerate 找到
    usage page `0xff43`(Logitech 私有通道)的那一個

## 分階段開發計畫

### Phase 0 — 裝置探測(唯讀,無風險)
- [ ] Enumerate 所有 vendor ID `046d` 的 HID 裝置,列出 path / usage page / usage
- [ ] 找出 HID++ 私有通道 interface
- [ ] 送出 root feature (`0x0000`) 查詢,印出完整 feature table
- [ ] **驗收標準:** 能穩定印出 PRO X2 SUPERSTRIKE 的 feature list,且能與
      `linux-superstrike -probe` 或 `hidpp-list-features` 的輸出做交叉比對

### Phase 1 — 標準功能讀取(唯讀)
- [ ] 電量讀取
- [ ] 目前 DPI 值讀取
- [ ] 目前 polling rate 讀取
- [ ] 按鍵映射現狀讀取
- [ ] **驗收標準:** CLI 指令 `superstrike-cli info` 印出上述所有資訊,數值與 G HUB
      畫面上顯示的一致

### Phase 2 — 標準功能寫入
- [ ] DPI 寫入
- [ ] Polling rate 寫入
- [ ] **驗收標準:** 寫入後透過 Phase 1 的讀取指令驗證數值已改變,且滑鼠行為確實改變

### Phase 3 — HITS 私有功能(高風險,需先比對 linux-superstrike 的逆向文件)
- [ ] 對照 `linux-superstrike` 的 `REVERSE_ENGINEERING.md`,找出 HITS 對應的
      feature index 與 payload 格式
- [ ] 讀取目前 actuation point / rapid trigger reset 設定
- [ ] 寫入 actuation point / rapid trigger reset 設定
- [ ] **驗收標準:** 能讀出與 G HUB 一致的數值;寫入後手感確實改變(主觀驗證)
- [ ] **風險註記:** 若協定與 linux-superstrike 記錄的不一致(韌體版本差異),
      需要重新走 Phase 0 的封包比對流程,不可貿然對裝置送出未驗證的寫入指令

### Phase 4(選用)— 按鍵重映射與 onboard profile
- [ ] 依 `linux-superstrike` 記錄的 profile sector 格式,讀取/寫入 onboard profile
- [ ] **風險註記:** onboard profile 涉及裝置韌體內部記憶體佈局,寫入錯誤格式可能導致
      裝置設定損毀(非永久性硬體損壞,但可能需要恢復原廠設定),**務必先完整驗證讀取路徑
      再進行寫入實驗**

## 安全原則(貫穿全專案)

1. **先讀不寫**:任何 feature 在確認能穩定「讀」出正確值之前,不要嘗試「寫」。
2. **每次寫入前都先讀一次現有值**,存下來,方便寫壞時人工用 G HUB(如果還裝著)復原。
3. **未知 feature index 不要亂猜 payload 硬送**,寫入類型的 HID++ command 送錯格式
   有極小機率讓裝置進入異常狀態(通常斷電重連可救回,但沒有絕對保證)。
4. 開發過程中建議保留一台備用滑鼠或確保能透過官方 G HUB 隨時恢復原廠設定作為 fallback。

## 專案結構建議

```
superstrike-cli/
├── cmd/
│   └── superstrike-cli/main.go       # CLI entrypoint
├── internal/
│   ├── hidpp/                        # HID++ 2.0 協定層(report 組裝/解析)
│   │   ├── report.go
│   │   ├── feature_table.go
│   │   └── root.go
│   ├── device/                       # 裝置探測與連線管理
│   │   └── enumerate.go
│   └── features/                     # 各功能模組(dpi, battery, hits, buttons...)
│       ├── dpi.go
│       ├── battery.go
│       └── hits.go
├── go.mod
└── README.md
```

## 給 Claude Code 的起手指令建議

啟動時建議先讓它做 Phase 0,並要求先產出一份 `PROTOCOL_NOTES.md` 記錄從
`linux-superstrike` 和 `libratbag` 讀到的協定細節,再開始寫程式——避免它憑空腦補
HID++ payload 格式。

範例起手 prompt:

> 參考 mclol0/linux-superstrike 這個專案(尤其是 REVERSE_ENGINEERING.md)以及
> libratbag/libratbag 的 hidpp20.h/hidpp20.c,先幫我整理一份 PROTOCOL_NOTES.md,
> 列出 PRO X2 SUPERSTRIKE 相關的 HID++ feature ID 對照表與 report payload 格式,
> 標註哪些是標準 HID++ 2.0 功能、哪些是這台機種私有的(尤其 HITS)。整理完之後,
> 依照這份 proposal 的 Phase 0 開始實作一個 Go CLI,先做唯讀的裝置探測與 feature
> table dump,不要碰任何寫入功能。
