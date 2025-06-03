package utils

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// Data определяет структуру для хранения данных о продукте
type Data struct {
	Hash  string
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

	maxRetries = 3
	baseDelay  = 1 * time.Second
	maxDelay   = 10 * time.Second
)

// Карта для перевода русских названий месяцев (в родительном падеже, как в примере) в числовой формат
var RussianMonths = map[string]string{
	"января":   "01",
	"февраля":  "02",
	"марта":    "03",
	"апреля":   "04",
	"мая":      "05",
	"июня":     "06",
	"июля":     "07",
	"августа":  "08",
	"сентября": "09",
	"октября":  "10",
	"ноября":   "11",
	"декабря":  "12",
}

func GetHTML(pageUrl string) (*goquery.Document, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{ // Настроим транспорт для лучшего управления соединениями
			MaxIdleConns:        100, // Больше неактивных соединений в пуле
			MaxIdleConnsPerHost: 10,  // Больше неактивных соединений на хост (важно для gazeta.ru)
			IdleConnTimeout:     90 * time.Second,
		},
	}
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(int64(baseDelay) * (1 << (attempt - 1))) // Экспоненциальная задержка: 1s, 2s, 4s...
			if delay > maxDelay {
				delay = maxDelay
			}
			// Добавляем джиттер (случайное небольшое отклонение), чтобы избежать "громоподобного стада"
			jitter := time.Duration(rand.Intn(1000)) * time.Millisecond
			time.Sleep(delay + jitter)
			fmt.Printf("%s[UTILS]%s[RETRY] Попытка #%d для %s после ошибки: %v%s\n", ColorBlue, ColorYellow, attempt+1, LimitString(pageUrl, 70), lastErr, ColorReset)
		}

		req, err := http.NewRequest("GET", pageUrl, nil)
		if err != nil {
			lastErr = fmt.Errorf("создание HTTP GET-запроса для %s: %w", pageUrl, err)
			// Эта ошибка обычно не требует повтора, но для консистентности можно оставить в цикле
			continue
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		// Можно добавить больше "человеческих" заголовков
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
		req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.8,en;q=0.7")
		req.Header.Set("Cache-Control", "max-age=0")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("выполнение HTTP GET-запроса к %s: %w", pageUrl, err)
			// Сетевые ошибки или таймауты клиента - определенно кандидаты на повтор
			continue
		}

		// Важно закрывать тело в каждой итерации, если был получен ответ
		bodyToClose := resp.Body
		defer func() {
			if bodyToClose != nil {
				bodyToClose.Close()
			}
		}()

		if resp.StatusCode != http.StatusOK {
			// Читаем тело, чтобы сервер не держал соединение и для логгирования
			_, _ = io.Copy(io.Discard, resp.Body) // Игнорируем ошибку чтения, т.к. статус уже не 200

			// Некоторые статусы (например, 404 Not Found) не должны вызывать повторы
			if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden || (resp.StatusCode >= 400 && resp.StatusCode < 500) {
				if attempt == 3 {
					return nil, fmt.Errorf("HTTP-запрос к %s вернул статус %d (%s) - не повторяем", pageUrl, resp.StatusCode, resp.Status)

				}
			}
			lastErr = fmt.Errorf("HTTP-запрос к %s вернул статус %d (%s) вместо 200 (OK)", pageUrl, resp.StatusCode, resp.Status)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			// Вот здесь ваша ошибка "stream error... INTERNAL_ERROR" будет поймана
			lastErr = fmt.Errorf("ошибка парсинга HTML со страницы %s: %w", pageUrl, err)
			// Если ошибка содержит "INTERNAL_ERROR" или "stream error", это хороший кандидат на повтор
			if strings.Contains(err.Error(), "INTERNAL_ERROR") || strings.Contains(err.Error(), "stream error") || strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "connection reset") {
				// Можно не закрывать bodyToClose здесь явно, т.к. defer сработает.
				// Но если бы мы хотели прочитать тело снова, его нужно было бы как-то "пересоздать" или не читать до конца.
				// В данном случае goquery.NewDocumentFromReader уже его прочитал (или пытался).
				bodyToClose = nil // Предотвращаем двойное закрытие, т.к. goquery мог уже закрыть
				resp.Body.Close() // Закрываем "оригинальное" тело, если goquery не справился
				continue
			}
			// Другие ошибки парсинга (например, невалидный HTML) повторять нет смысла
			return nil, lastErr
		}
		return doc, nil // Успех
	}
	return nil, fmt.Errorf("превышено количество попыток (%d) для %s: %w", maxRetries, pageUrl, lastErr)
}

func GetJSON(pageUrl string) ([]byte, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{ // Настроим транспорт для лучшего управления соединениями
			MaxIdleConns:        100, // Больше неактивных соединений в пуле
			MaxIdleConnsPerHost: 10,  // Больше неактивных соединений на хост (важно для gazeta.ru)
			IdleConnTimeout:     90 * time.Second,
		},
	}
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

	bodyBytes, err := io.ReadAll(resp.Body) // Используем ReadAll из utils или io
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения тела JSON ответа с %s: %w", pageUrl, err)
	}
	return bodyBytes, nil
}

func ExtractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.Join(strings.Fields(n.Data), " ")
	}
	if n.Type == html.ElementNode &&
		(n.Data == "script" || n.Data == "style" || n.Data == "noscript" || n.Data == "iframe" || n.Data == "svg" || n.Data == "img" || n.Data == "video" || n.Data == "audio" || n.Data == "figure" || n.Data == "picture") {
		return "" // Игнорируем эти теги и их содержимое
	}

	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractedChildText := ExtractText(c)
		if extractedChildText != "" {
			if sb.Len() > 0 && !strings.HasSuffix(sb.String(), " ") && !strings.HasPrefix(extractedChildText, " ") {
				sb.WriteString(" ")
			}
			sb.WriteString(extractedChildText)
		}
	}
	return sb.String()
}

func GetAttribute(h *html.Node, key string) (string, bool) {
	if h == nil {
		return "", false
	}
	for _, attr := range h.Attr {
		if attr.Key == key {
			return attr.Val, true
		}
	}
	return "", false
}

func HasAllClasses(h *html.Node, targetClasses string) bool {
	if h == nil {
		return false
	}
	classAttr, ok := GetAttribute(h, "class")
	if !ok {
		return false
	}
	actualClasses := strings.Fields(classAttr)
	expectedClasses := strings.Fields(targetClasses)
	if len(expectedClasses) == 0 {
		return true
	}
	for _, expected := range expectedClasses {
		found := false
		for _, actual := range actualClasses {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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
	if len(s) <= length {
		return s
	}
	if length < 3 { // Если длина слишком мала для "..."
		if length <= 0 {
			return ""
		}
		return s[:length]
	}
	return s[:length-3] + "..."
}

// generateURLForPastDate генерирует URL для даты N дней назад
func GenerateURLForPastDate(daysAgo int) time.Time {
	today := time.Now()
	pastDate := today.AddDate(0, 0, -daysAgo) // Вычитаем дни
	return pastDate
}

// Альтернативная, более короткая версия generateURLForDate с использованием fmt.Sprintf
func GenerateURLForDateFormatted(date time.Time) string {
	year := fmt.Sprintf("%d", date.Year())
	month := fmt.Sprintf("%02d", date.Month()) // %02d - означает двузначное число, с ведущим нулем если нужно
	day := fmt.Sprintf("%02d", date.Day())     // %02d - означает двузначное число, с ведущим нулем если нужно
	return fmt.Sprintf("%s.%s.%s", day, month, year)
}

func ProgressBar(title string, body string, pageStatusMessage string, statusMessageColor string, i int, totalLinks int) {
	progressBarLength := 40
	statusTextWidth := 90

	percent := int((float64(i+1) / float64(totalLinks)) * 100)
	completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
	if completedChars < 0 {
		completedChars = 0
	}
	if completedChars > progressBarLength {
		completedChars = progressBarLength
	}

	bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
	countStr := fmt.Sprintf("(%d/%d) ", i+1, totalLinks)

	// Рассчитываем доступную ширину для сообщения статуса
	remainingWidthForMessage := statusTextWidth - len(countStr)
	if remainingWidthForMessage < 10 { // Минимальная ширина для сообщения
		remainingWidthForMessage = 10
	}

	// Обрезаем сообщение статуса, если оно слишком длинное
	displayStatusMessage := pageStatusMessage
	if len(displayStatusMessage) > remainingWidthForMessage {
		displayStatusMessage = LimitString(displayStatusMessage, remainingWidthForMessage)
	}

	// Формируем полную строку статуса, выравнивая ее пробелами
	fullStatusText := countStr + displayStatusMessage
	if len(fullStatusText) < statusTextWidth {
		fullStatusText += strings.Repeat(" ", statusTextWidth-len(fullStatusText))
	} else if len(fullStatusText) > statusTextWidth { // На всякий случай, если LimitString не сработал идеально
		fullStatusText = fullStatusText[:statusTextWidth]
	}

	fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, statusMessageColor, fullStatusText, ColorReset)
}
