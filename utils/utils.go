// utils/utils.go
package utils

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
	"golang.org/x/net/html/charset"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	// "golang.org/x/net/html" // Закомментировано, так как не используется в этой функции напрямую
)

// Data определяет структуру для хранения данных о продукте
type Data struct {
	Hash  string
	Site  string
	Href  string
	Title string
	Body  string
	Date  time.Time
	Tags  []string
}

const (
	ColorReset  = "\033[0m"
	ColorGreen  = "\033[32m"
	ColorRed    = "\033[31m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorCyan   = "\033[36m"

	maxRetries = 3
	baseDelay  = 1 * time.Second
	maxDelay   = 10 * time.Second
)

var RussianMonths = map[string]string{
	// Нижний регистр (склонения)
	"января": "01", "февраля": "02", "марта": "03", "апреля": "04", "мая": "05", "июня": "06",
	"июля": "07", "августа": "08", "сентября": "09", "октября": "10", "ноября": "11", "декабря": "12",
	// Нижний регистр (именительный падеж)
	"январь": "01", "февраль": "02", "март": "03", "апрель": "04", "май": "05", "июнь": "06",
	"июль": "07", "август": "08", "сентябрь": "09", "октябрь": "10", "ноябрь": "11", "декабрь": "12",

	// Верхний регистр (склонения)
	"ЯНВАРЯ": "01", "ФЕВРАЛЯ": "02", "МАРТА": "03", "АПРЕЛЯ": "04", "МАЯ": "05", "ИЮНЯ": "06",
	"ИЮЛЯ": "07", "АВГУСТА": "08", "СЕНТЯБРЯ": "09", "ОКТЯБРЯ": "10", "НОЯБРЯ": "11", "ДЕКАБРЯ": "12",
	// Верхний регистр (именительный падеж)
	"ЯНВАРЬ": "01", "ФЕВРАЛЬ": "02", "МАРТ": "03", "АПРЕЛЬ": "04", "МАЙ": "05", "ИЮНЬ": "06",
	"ИЮЛЬ": "07", "АВГУСТ": "08", "СЕНТЯБРЬ": "09", "ОКТЯБРЬ": "10", "НОЯБРЬ": "11", "ДЕКАБРЬ": "12",
}

// russianMonthsLife ДОЛЖЕН содержать английские НАЗВАНИЯ месяцев
var RussianMonthsLife = map[string]string{
	"января":   "January",
	"февраля":  "February",
	"марта":    "March",
	"апреля":   "April",
	"мая":      "May",
	"июня":     "June",
	"июля":     "July",
	"августа":  "August",
	"сентября": "September",
	"октября":  "October",
	"ноября":   "November",
	"декабря":  "December",
}

// DbConn представляет пул соединений
var DbConn *sql.DB

// DSN (Data Source Name) - строка подключения БЕЗ пароля (текущие настройки)
const DSN = "user=postgres dbname=parsing_media_db host=localhost port=5432 sslmode=disable"

// Функция InitDB: Инициализирует соединение с базой данных
func InitDB() error {
	var err error
	DbConn, err = sql.Open("postgres", DSN)
	if err != nil {
		return fmt.Errorf("ошибка открытия БД: %w", err)
	}

	if err = DbConn.Ping(); err != nil {
		return fmt.Errorf("ошибка проверки соединения с БД: %w", err)
	}

	// Рекомендуемые настройки
	DbConn.SetMaxOpenConns(25)
	DbConn.SetMaxIdleConns(5)
	DbConn.SetConnMaxLifetime(5 * time.Minute)

	return nil
}

// Функция SaveData: Сохраняет данные в БД
func SaveData(products []Data) {
	if DbConn == nil {
		fmt.Printf("%s[DB] Соединение с БД не инициализировано.%s\n", ColorRed, ColorReset)
		return
	}

	if len(products) == 0 {
		return
	}

	//fmt.Printf("%s[DB] Сохранение %d записей в БД...%s\n", ColorCyan, len(products), ColorReset)

	// SQL: Вставка, которая игнорирует дубликаты по хешу (ON CONFLICT (hash) DO NOTHING)
	sqlStatement := `
    INSERT INTO articles (hash, site, href, title, body, date, tags)
    VALUES ($1, $2, $3, $4, $5, $6, $7)
    ON CONFLICT (hash) DO NOTHING;`

	tx, err := DbConn.Begin()
	if err != nil {
		fmt.Printf("%s[DB][ERROR] Ошибка начала транзакции: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	stmt, err := tx.Prepare(sqlStatement)
	if err != nil {
		fmt.Printf("%s[DB][ERROR] Ошибка подготовки запроса: %v%s\n", ColorRed, err, ColorReset)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	insertedCount := 0
	for _, p := range products {
		_, err = stmt.Exec(p.Hash, p.Site, p.Href, p.Title, p.Body, p.Date, pq.Array(p.Tags))
		if err != nil {
			// Если ошибка - дубликат по href, просто пропускаем
			if !strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
				fmt.Printf("%s[DB][WARN] Ошибка вставки %s: %v%s\n", ColorYellow, LimitString(p.Title, 40), err, ColorReset)
			}
			continue
		}
		insertedCount++
	}

	err = tx.Commit()
	if err != nil {
		fmt.Printf("%s[DB][ERROR] Ошибка фиксации транзакции: %v%s\n", ColorRed, err, ColorReset)
		return
	}

	//fmt.Printf("%s[DB] Успешно сохранено %d новых записей (из %d) в БД.%s\n", ColorGreen, insertedCount, len(products), ColorReset)
}

func (d *Data) Hashing() (string, error) {
	var builder strings.Builder

	builder.WriteString(d.Title)
	builder.WriteString(d.Body)
	builder.WriteString(strconv.FormatInt(d.Date.Unix(), 10))

	hasher := sha256.New()

	if _, err := io.WriteString(hasher, builder.String()); err != nil {
		return "", fmt.Errorf("ошибка записи данных в хешер: %w", err)
	}
	hashBytes := hasher.Sum(nil)

	return fmt.Sprintf("%x", hashBytes), nil
}

func GetHTMLForClient(client *http.Client, pageUrl string) (*goquery.Document, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			var currentDelay time.Duration
			if attempt == 1 {
				currentDelay = baseDelay
			} else {
				currentDelay = baseDelay * time.Duration(1<<(attempt-1)) * time.Duration(rand.Intn(500)+500) / 1000
			}
			if currentDelay > maxDelay {
				currentDelay = maxDelay
			}
			jitter := time.Duration(rand.Intn(500)) * time.Millisecond
			time.Sleep(currentDelay + jitter)
			fmt.Printf("%s[UTILS]%s[RETRY] Попытка #%d для %s после ошибки: %v%s\n", ColorBlue, ColorYellow, attempt+1, LimitString(pageUrl, 70), lastErr, ColorReset)
		}

		req, err := http.NewRequest("GET", pageUrl, nil)
		if err != nil {
			lastErr = fmt.Errorf("создание HTTP GET-запроса для %s: %w", pageUrl, err)
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("выполнение HTTP GET-запроса к %s: %w", pageUrl, err)
			continue
		}

		bodyToClose := resp.Body
		defer func(b io.ReadCloser) {
			if b != nil {
				io.Copy(io.Discard, b)
				b.Close()
			}
		}(bodyToClose)

		contentType := resp.Header.Get("Content-Type")
		// fmt.Printf("%s[UTILS]%s[INFO] URL: %s, Status: %s, Content-Type: %s%s\n", ColorBlue, ColorYellow, LimitString(pageUrl, 70), resp.Status, contentType, ColorReset) // Убрано

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || (resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests) {
				bodyToClose = nil
				if attempt == 3 {
					return nil, fmt.Errorf("HTTP-запрос к %s вернул статус %d (%s) - не повторяем", pageUrl, resp.StatusCode, resp.Status)
				}
			}
			lastErr = fmt.Errorf("HTTP-запрос к %s вернул статус %d (%s) вместо 200 (OK)", pageUrl, resp.StatusCode, resp.Status)
			bodyToClose = nil
			// resp.Body.Close() // defer func сделает это, если bodyToClose не nil. Здесь он уже nil.
			continue
		}

		bodyBytes, err := io.ReadAll(bodyToClose)
		bodyToClose = nil
		if err != nil {
			lastErr = fmt.Errorf("ошибка чтения тела ответа с %s: %w", pageUrl, err)
			continue
		}

		var readerForDoc io.Reader = bytes.NewReader(bodyBytes)
		// finalEncodingName := "utf-8 (assumed by goquery or from meta tag)" // Убрано, так как используется только для лога

		if contentType != "" && !strings.Contains(strings.ToLower(contentType), "charset=utf-8") {
			var determinedEnc encoding.Encoding
			var encName string
			// var encCertain bool // Убрано, не используется без логов

			if strings.Contains(strings.ToLower(contentType), "charset=windows-1251") {
				determinedEnc = charmap.Windows1251
				encName = "windows-1251"
				// encCertain = true // Убрано
			} else {
				e, name, _ := charset.DetermineEncoding(bodyBytes, contentType) // certain не используется
				determinedEnc = e
				encName = name
				// encCertain = certain // Убрано
			}

			if determinedEnc != nil && encName != "utf-8" {
				readerForDoc = transform.NewReader(bytes.NewReader(bodyBytes), determinedEnc.NewDecoder())
				// finalEncodingName = encName + " (decoded to UTF-8)" // Убрано
			} else if strings.Contains(pageUrl, "interfax.ru") && !strings.Contains(strings.ToLower(contentType), "charset=") {
				readerForDoc = transform.NewReader(bytes.NewReader(bodyBytes), charmap.Windows1251.NewDecoder())
				// finalEncodingName = "windows-1251 (forced for Interfax, charset absent)" // Убрано
			}
		} else if contentType == "" {
			e, name, _ := charset.DetermineEncoding(bodyBytes, "") // certain не используется
			// finalEncodingName = fmt.Sprintf("%s (certain: %t, Content-Type was empty)", name, certain) // Убрано
			if e != nil && name != "utf-8" {
				readerForDoc = transform.NewReader(bytes.NewReader(bodyBytes), e.NewDecoder())
			} else if e == nil && strings.Contains(pageUrl, "interfax.ru") {
				readerForDoc = transform.NewReader(bytes.NewReader(bodyBytes), charmap.Windows1251.NewDecoder())
				// finalEncodingName = "windows-1251 (forced for Interfax, Content-Type empty)" // Убрано
			}
		} else {
			// finalEncodingName = "utf-8 (from Content-Type or assumed)" // Убрано
		}

		doc, err := goquery.NewDocumentFromReader(readerForDoc)
		if err != nil {
			// Оставляем вывод ошибки парсинга, так как это может быть важно
			lastErr = fmt.Errorf("ошибка парсинга HTML со страницы %s: %w", pageUrl, err)
			if strings.Contains(err.Error(), "INTERNAL_ERROR") || strings.Contains(err.Error(), "stream error") || strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "connection reset") {
				continue
			}
			return nil, lastErr
		}
		return doc, nil
	}
	return nil, fmt.Errorf("превышено количество попыток (%d) для %s: %w", maxRetries, pageUrl, lastErr)
}

// --- Остальные функции из utils.go ---
func GetJSONForClient(client *http.Client, pageUrl string) ([]byte, error) {
	req, err := http.NewRequest("GET", pageUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("создание HTTP GET-запроса для JSON API %s: %w", pageUrl, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение HTTP GET-запроса к JSON API %s: %w", pageUrl, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP-запрос к JSON API %s вернул статус %d (%s) вместо 200 (OK)", pageUrl, resp.StatusCode, resp.Status)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения тела JSON ответа с %s: %w", pageUrl, err)
	}
	return bodyBytes, nil
}

func FormatDuration(d time.Duration) string {
	d = d.Round(time.Millisecond)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.3fs", d.Seconds())
	}
	minutes := int64(d.Minutes())
	remainingSeconds := d - (time.Duration(minutes) * time.Minute)
	secondsWithMillis := remainingSeconds.Seconds()
	return fmt.Sprintf("%dm %.3fs", minutes, secondsWithMillis)
}

func LimitString(s string, length int) string {
	runes := []rune(s) // Используем руны для корректной обрезки многобайтовых символов
	if len(runes) <= length {
		return s
	}
	if length < 3 {
		if length <= 0 {
			return ""
		}
		return string(runes[:length])
	}
	return string(runes[:length-3]) + "..."
}
