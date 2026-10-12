// Package xlsx Excel (.xlsx) dosyası yazar. Yalnız programın raporları için
// gereken kadarı vardır: birden çok sayfa, kalın başlık, para/tarih biçimi,
// sütun genişliği, dondurulmuş başlık satırı ve süzgeç.
package xlsx

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Hücre biçimleri (styles.xml içindeki cellXfs sırası).
const (
	StyleNormal = iota
	StyleHeader
	StyleMoney
	StyleDate
	StyleInt
	StylePercent
	StyleTitle
	StyleMoneyBold
	StyleDateTime
	StyleBold
	StyleIntBold
	StyleTime
)

// Cell bir hücredir. V: string, int, int64, float64, time.Time ya da nil.
type Cell struct {
	V     any
	Style int
}

// Text yazı hücresidir.
func Text(s string) Cell { return Cell{V: s} }

// Bold kalın yazı hücresidir.
func Bold(s string) Cell { return Cell{V: s, Style: StyleBold} }

// Title sayfa başlığıdır.
func Title(s string) Cell { return Cell{V: s, Style: StyleTitle} }

// Header sütun başlığıdır.
func Header(s string) Cell { return Cell{V: s, Style: StyleHeader} }

// Int tam sayı hücresidir.
func Int(n int64) Cell { return Cell{V: n, Style: StyleInt} }

// Money kuruş cinsinden tutarı lira olarak yazar.
func Money(kurus int64) Cell { return Cell{V: float64(kurus) / 100, Style: StyleMoney} }

// MoneyBold kalın tutar hücresidir (toplam satırı).
func MoneyBold(kurus int64) Cell { return Cell{V: float64(kurus) / 100, Style: StyleMoneyBold} }

// Percent yüzde hücresidir (0.25 → %25).
func Percent(f float64) Cell { return Cell{V: f, Style: StylePercent} }

// Date tarih hücresidir.
func Date(t time.Time) Cell { return Cell{V: t, Style: StyleDate} }

// DateTime tarih-saat hücresidir.
func DateTime(t time.Time) Cell { return Cell{V: t, Style: StyleDateTime} }

// Clock saat hücresidir.
func Clock(t time.Time) Cell { return Cell{V: t, Style: StyleTime} }

// Empty boş hücredir.
func Empty() Cell { return Cell{} }

// Sheet bir çalışma sayfasıdır.
type Sheet struct {
	Name   string
	Widths []float64 // sütun genişlikleri (karakter)
	Rows   [][]Cell
	// HeaderRow > 0 ise o satır (1'den başlar) başlıktır: üstü dondurulur ve süzgeç eklenir.
	HeaderRow int
	// FilterLast süzgecin son satırıdır (0: son satır). Toplam satırı süzgeç dışında kalsın diye.
	FilterLast int
}

func colName(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

// excelSerial Excel tarih sayısıdır (1899-12-30'dan beri gün).
func excelSerial(t time.Time) float64 {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	u := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
	return u.Sub(base).Hours() / 24
}

func cleanXML(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "?")
	}
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || (r >= 0x20 && r <= 0xD7FF) || (r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF) {
			return r
		}
		return -1
	}, s)
}

func escape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(cleanXML(s)))
	return b.String()
}

func formatNum(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

const header = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// sheetName Excel'in kabul ettiği sayfa adı yapar (en fazla 31 karakter, []:*?/\ yok).
func sheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '-'
		}
		return r
	}, s)
	if utf8.RuneCountInString(s) > 31 {
		s = string([]rune(s)[:31])
	}
	if strings.TrimSpace(s) == "" {
		s = "Sayfa"
	}
	return s
}

// Write sayfaları .xlsx olarak yazar.
func Write(w io.Writer, sheets []Sheet) error {
	if len(sheets) == 0 {
		return fmt.Errorf("sayfa yok")
	}
	zw := zip.NewWriter(w)
	put := func(name, body string) error {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		_, err = io.WriteString(f, body)
		return err
	}
	var ct strings.Builder
	ct.WriteString(header + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>` +
		`<Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>`)
	for i := range sheets {
		fmt.Fprintf(&ct, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i+1)
	}
	ct.WriteString(`</Types>`)
	if err := put("[Content_Types].xml", ct.String()); err != nil {
		return err
	}
	if err := put("_rels/.rels", header+`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`+
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>`+
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>`+
		`<Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>`+
		`</Relationships>`); err != nil {
		return err
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	if err := put("docProps/core.xml", header+`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">`+
		`<dc:creator>Halis Adisyon</dc:creator><dcterms:created xsi:type="dcterms:W3CDTF">`+now+`</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">`+now+`</dcterms:modified></cp:coreProperties>`); err != nil {
		return err
	}
	if err := put("docProps/app.xml", header+`<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"><Application>Halis Adisyon</Application></Properties>`); err != nil {
		return err
	}

	var wb, rels strings.Builder
	wb.WriteString(header + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><bookViews><workbookView activeTab="0"/></bookViews><sheets>`)
	rels.WriteString(header + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	var defined strings.Builder
	used := map[string]bool{}
	names := make([]string, len(sheets))
	for i, sh := range sheets {
		n := sheetName(sh.Name)
		for k := 2; used[strings.ToLower(n)]; k++ {
			n = sheetName(fmt.Sprintf("%s %d", sh.Name, k))
		}
		used[strings.ToLower(n)] = true
		names[i] = n
		fmt.Fprintf(&wb, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, escape(n), i+1, i+1)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i+1, i+1)
		if ref := filterRef(sh); ref != "" {
			fmt.Fprintf(&defined, `<definedName name="_xlnm._FilterDatabase" localSheetId="%d" hidden="1">'%s'!%s</definedName>`, i,
				escape(strings.ReplaceAll(n, "'", "''")), absRef(ref))
		}
	}
	wb.WriteString(`</sheets>`)
	if defined.Len() > 0 {
		wb.WriteString(`<definedNames>` + defined.String() + `</definedNames>`)
	}
	wb.WriteString(`</workbook>`)
	fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`, len(sheets)+1)
	if err := put("xl/workbook.xml", wb.String()); err != nil {
		return err
	}
	if err := put("xl/_rels/workbook.xml.rels", rels.String()); err != nil {
		return err
	}
	if err := put("xl/styles.xml", styles); err != nil {
		return err
	}
	for i, sh := range sheets {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), Method: zip.Deflate, Modified: time.Now()})
		if err != nil {
			return err
		}
		bw := bufio.NewWriter(f)
		writeSheet(bw, sh, i == 0)
		if err := bw.Flush(); err != nil {
			return err
		}
	}
	return zw.Close()
}

func maxCols(sh Sheet) int {
	n := len(sh.Widths)
	for _, r := range sh.Rows {
		if len(r) > n {
			n = len(r)
		}
	}
	return n
}

// filterRef süzgeç alanıdır: başlık satırından son satıra.
func filterRef(sh Sheet) string {
	if sh.HeaderRow <= 0 || sh.HeaderRow > len(sh.Rows) {
		return ""
	}
	cols := len(sh.Rows[sh.HeaderRow-1])
	if cols == 0 {
		return ""
	}
	last := len(sh.Rows)
	if sh.FilterLast > 0 && sh.FilterLast < last {
		last = sh.FilterLast
	}
	if last <= sh.HeaderRow {
		last = sh.HeaderRow + 1
	}
	return fmt.Sprintf("A%d:%s%d", sh.HeaderRow, colName(cols-1), last)
}

func absRef(ref string) string {
	parts := strings.Split(ref, ":")
	for i, p := range parts {
		j := strings.IndexAny(p, "0123456789")
		parts[i] = "$" + p[:j] + "$" + p[j:]
	}
	return strings.Join(parts, ":")
}

func writeSheet(w *bufio.Writer, sh Sheet, first bool) {
	w.WriteString(header + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	n := maxCols(sh)
	if len(sh.Rows) > 0 && n > 0 {
		fmt.Fprintf(w, `<dimension ref="A1:%s%d"/>`, colName(n-1), len(sh.Rows))
	}
	w.WriteString(`<sheetViews><sheetView workbookViewId="0"`)
	if first {
		w.WriteString(` tabSelected="1"`)
	}
	w.WriteString(`>`)
	if sh.HeaderRow > 0 {
		fmt.Fprintf(w, `<pane ySplit="%d" topLeftCell="A%d" activePane="bottomLeft" state="frozen"/><selection pane="bottomLeft" activeCell="A%d" sqref="A%d"/>`,
			sh.HeaderRow, sh.HeaderRow+1, sh.HeaderRow+1, sh.HeaderRow+1)
	}
	w.WriteString(`</sheetView></sheetViews><sheetFormatPr defaultRowHeight="15"/>`)
	if len(sh.Widths) > 0 {
		w.WriteString(`<cols>`)
		for i, wd := range sh.Widths {
			if wd <= 0 {
				wd = 10
			}
			fmt.Fprintf(w, `<col min="%d" max="%d" width="%s" customWidth="1"/>`, i+1, i+1, formatNum(wd))
		}
		w.WriteString(`</cols>`)
	}
	w.WriteString(`<sheetData>`)
	for ri, row := range sh.Rows {
		fmt.Fprintf(w, `<row r="%d">`, ri+1)
		for ci, c := range row {
			ref := colName(ci) + strconv.Itoa(ri+1)
			s := ""
			if c.Style != StyleNormal {
				s = fmt.Sprintf(` s="%d"`, c.Style)
			}
			switch v := c.V.(type) {
			case nil:
				if s != "" {
					fmt.Fprintf(w, `<c r="%s"%s/>`, ref, s)
				}
			case string:
				sp := ""
				if v != strings.TrimSpace(v) {
					sp = ` xml:space="preserve"`
				}
				fmt.Fprintf(w, `<c r="%s"%s t="inlineStr"><is><t%s>%s</t></is></c>`, ref, s, sp, escape(v))
			case int:
				fmt.Fprintf(w, `<c r="%s"%s><v>%d</v></c>`, ref, s, v)
			case int64:
				fmt.Fprintf(w, `<c r="%s"%s><v>%d</v></c>`, ref, s, v)
			case float64:
				fmt.Fprintf(w, `<c r="%s"%s><v>%s</v></c>`, ref, s, formatNum(v))
			case time.Time:
				if v.IsZero() {
					continue
				}
				fmt.Fprintf(w, `<c r="%s"%s><v>%s</v></c>`, ref, s, formatNum(excelSerial(v)))
			case bool:
				b := 0
				if v {
					b = 1
				}
				fmt.Fprintf(w, `<c r="%s"%s t="b"><v>%d</v></c>`, ref, s, b)
			default:
				fmt.Fprintf(w, `<c r="%s"%s t="inlineStr"><is><t>%s</t></is></c>`, ref, s, escape(fmt.Sprint(v)))
			}
		}
		w.WriteString(`</row>`)
	}
	w.WriteString(`</sheetData>`)
	if ref := filterRef(sh); ref != "" {
		fmt.Fprintf(w, `<autoFilter ref="%s"/>`, ref)
	}
	w.WriteString(`<pageMargins left="0.5" right="0.5" top="0.6" bottom="0.6" header="0.3" footer="0.3"/>`)
	w.WriteString(`</worksheet>`)
}

// styles hücre biçimleridir; sıra yukarıdaki Style sabitleriyle aynıdır.
const styles = header + `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
	`<numFmts count="4">` +
	`<numFmt numFmtId="164" formatCode="#,##0.00\ &quot;₺&quot;"/>` +
	`<numFmt numFmtId="165" formatCode="dd\.mm\.yyyy"/>` +
	`<numFmt numFmtId="166" formatCode="dd\.mm\.yyyy\ hh:mm"/>` +
	`<numFmt numFmtId="167" formatCode="hh:mm"/>` +
	`</numFmts>` +
	`<fonts count="3">` +
	`<font><sz val="11"/><name val="Calibri"/><family val="2"/></font>` +
	`<font><b/><sz val="11"/><name val="Calibri"/><family val="2"/></font>` +
	`<font><b/><sz val="15"/><name val="Calibri"/><family val="2"/></font>` +
	`</fonts>` +
	`<fills count="3"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill>` +
	`<fill><patternFill patternType="solid"><fgColor rgb="FFF1E8DC"/><bgColor indexed="64"/></patternFill></fill></fills>` +
	`<borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border>` +
	`<border><left/><right/><top/><bottom style="thin"><color rgb="FF8C7B68"/></bottom><diagonal/></border></borders>` +
	`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
	`<cellXfs count="12">` +
	`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
	`<xf numFmtId="0" fontId="1" fillId="2" borderId="1" xfId="0" applyFont="1" applyFill="1" applyBorder="1"/>` +
	`<xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="3" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="10" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1"/>` +
	`<xf numFmtId="164" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/>` +
	`<xf numFmtId="166" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` +
	`<xf numFmtId="3" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/>` +
	`<xf numFmtId="167" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`</cellXfs>` +
	`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
	`</styleSheet>`
