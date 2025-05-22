package kommers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Data определяет структуру для хранения данных о продукте
type Data struct {
	Title string
	Body  string
}

// Константы (Цветовые константы ANSI)
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"

	quantityLinks     = 100
	kommersURL        = "https://www.kommersant.ru/"
	kommersURLNews    = "https://www.kommersant.ru/lenta"
	kommersURLNewPage = "https://www.kommersant.ru/lenta?page=%d"
)

func KommersMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", colorYellow, colorReset)
	_ = parsingLinks()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", colorYellow, formatDuration(totalElapsedTime), colorReset)
}

func parsingLinks() []Data {
	var found_links []string // Срез (массив) для хранения найденных ссылок.

	var extractLinks func(*html.Node) // Рекурсивная функция обхода HTML для получения ссылок.
	extractLinks = func(h *html.Node) {
		if h.Type == html.ElementNode && h.Data == "a" { // Проверка того, является ли узел элементом <a>.
			var hrefValue string    // href="[значение, которое будет записано в переменную]".
			var classValue string   // class="[значение, которое будет записано в переменную]".
			isClassCorrect := false // Является ли class="" верным (который нам нужен).

			for _, attr := range h.Attr { // Ищем атрибуты "href" и "class":
				if attr.Key == "href" {
					hrefValue = attr.Val
				}
				if attr.Key == "class" {
					classValue = attr.Val
				}
			}

			if classValue == "uho__link uho__link--overlay" { // Проверяем, соответствует ли класс искомому.
				isClassCorrect = true
			}

			if isClassCorrect && hrefValue != "" { // Если класс совпадает и есть атрибут href — добавляем ссылку.
				//isDuplicate := false // Добавляем ссылку, если она еще не была добавлена.
				//for _, l := range found_links {
				//	if l == hrefValue {
				//		isDuplicate = true
				//		break
				//	}
				//}

				if !strings.HasPrefix(hrefValue, "https://") { // Проверка того, что это ссылка на «Lenta.ru».
					found_links = append(found_links, fmt.Sprint("https://www.kommersant.ru"+hrefValue))
				}
			}
		}

		if len(found_links) < quantityLinks {
			for c := h.FirstChild; c != nil; c = c.NextSibling { // Рекурсивно обходим всех потомков текущего узла.
				extractLinks(c)
			}
		}
	}

	fmt.Println("\nНачало парсинга ссылок...")

	// Получаем HTML-документ ленты новостей
	doc, err := getHTML(kommersURL)
	if err != nil {
		fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", kommersURL, err)
	}

	extractLinks(doc)
	progressBarLength := 40

	// Цикл для загрузки ссылок из дополнительных страниц
	for pageNumber := 1; len(found_links) < quantityLinks; pageNumber++ {

		// Парсинг доп ссылки
		doc, err := getHTML("https://www.kommersant.ru/lenta?page=" + fmt.Sprint(pageNumber))
		if err != nil {
			fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", kommersURL, err)
		}

		extractLinks(doc)

		// Расчет процента выполнения для прогресс-бара
		percent := int((float64(len(found_links)) / float64(quantityLinks)) * 100)
		// Расчет количества символов '█' для заполненной части прогресс-бара
		completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
		// Коррекция, чтобы completedChars не выходил за пределы длины прогресс-бара
		if completedChars < 0 {
			completedChars = 0
		}
		if completedChars > progressBarLength {
			completedChars = progressBarLength
		}

		// Формирование строки прогресс-бара: '█' для выполненной части, '-' для оставшейся
		bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
		// Формирование строки счетчика обработанных ссылок (например, "(10/100) ")
		countStr := fmt.Sprintf("(%d/%d) ", len(found_links), quantityLinks)

		// Выводим прогресс-бар, процент выполнения и статусное сообщение
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, colorGreen, countStr, colorReset)
	}

	// Выводим количество ссылок
	if len(found_links) > 0 {
		fmt.Printf("\n\nНайдено %d уникальных ссылок на статьи\n", len(found_links))
	} else {
		fmt.Printf("\nНе найдено ссылок с классом '%s' на странице %s.\n", "uho__link uho__link--overlay", kommersURL)
	}

	return parsingPage(found_links)
}

func parsingPage(links []string) []Data {
	var products []Data
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Println("Нет ссылок для парсинга.")
		return products
	} else {
		fmt.Println("\nНачало парсинга статей...")
	}

	progressBarLength := 40
	statusTextWidth := 80

	for i, URL := range links {
		var title, body string
		var pageStatusMessage string
		var statusMessageColor = colorReset

		doc, err := getHTML(URL)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", limitString(err.Error(), 50))
			statusMessageColor = colorRed // Ошибка - красный цвет
		} else {
			var get_data func(*html.Node)
			get_data = func(h *html.Node) {
				if h.Type == html.ElementNode {
					var classValue string
					for _, attr := range h.Attr {
						if attr.Key == "class" {
							classValue = attr.Val
							break
						}
					}

					if classValue == "doc_header__name js-search-mark" {
						if title == "" {
							title = strings.TrimSpace(extractText(h))
						}
					} else if classValue == "doc__text" {
						currentTextPart := strings.TrimSpace(extractText(h))
						if currentTextPart != "" {
							if body != "" {
								body += "\n"
							}
							body += currentTextPart
						}
					}
				}
				for c := h.FirstChild; c != nil; c = c.NextSibling {
					get_data(c)
				}
			}
			get_data(doc)

			if title != "" || body != "" {
				products = append(products, Data{Title: title, Body: body})
				pageStatusMessage = fmt.Sprintf("Успех: %s", limitString(title, 50))
				statusMessageColor = colorGreen // Успех - зеленый
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", limitString(URL, 50))
				statusMessageColor = colorRed // Нет данных - красный
			}
		}

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
		remainingWidthForMessage := statusTextWidth - len(countStr)
		if remainingWidthForMessage < 10 {
			remainingWidthForMessage = 10
		}

		if len(pageStatusMessage) > remainingWidthForMessage {
			pageStatusMessage = pageStatusMessage[:remainingWidthForMessage-3] + "..."
		}

		fullStatusText := countStr + pageStatusMessage
		if len(fullStatusText) < statusTextWidth {
			fullStatusText += strings.Repeat(" ", statusTextWidth-len(fullStatusText))
		} else if len(fullStatusText) > statusTextWidth {
			fullStatusText = fullStatusText[:statusTextWidth]
		}

		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, statusMessageColor, fullStatusText, colorReset)
	}

	fmt.Println(strings.Repeat(" ", progressBarLength+statusTextWidth+15))
	fmt.Printf("Парсинг завершен. Собрано %d статей.\n", len(products))

	//fmt.Println("\n--- Собранные данные ---")
	if len(products) == 0 {
		fmt.Println("\nНе удалось собрать данные ни с одной страницы.")
	}

	//for idx, product := range products {
	//	fmt.Printf("\nСтатья #%d\n", idx+1)
	//	fmt.Printf("Заголовок: %s\n", product.Title)
	//	fmt.Printf("Тело:\n%s\n", product.Body)
	//	fmt.Println(strings.Repeat("-", 100))
	//}

	return products
}

func getHTML(pageUrl string) (*html.Node, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	req, err := http.NewRequest("GET", pageUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("создание HTTP GET-запроса для %s: %w", pageUrl, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("выполнение HTTP GET-запроса к %s: %w", pageUrl, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP-запрос к %s вернул статус %d (%s) вместо 200 (OK)", pageUrl, resp.StatusCode, resp.Status)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("парсинг HTML со страницы %s: %w", pageUrl, err)
	}
	return doc, nil
}

func formatDuration(d time.Duration) string {
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

func getAttribute(h *html.Node, key string) (string, bool) {
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

func hasAllClasses(h *html.Node, targetClasses string) bool {
	if h == nil {
		return false
	}
	classAttr, ok := getAttribute(h, "class")
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

func limitString(s string, length int) string {
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

func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.Join(strings.Fields(n.Data), " ")
	}
	if n.Type == html.ElementNode &&
		(n.Data == "script" || n.Data == "style" || n.Data == "noscript" || n.Data == "iframe" || n.Data == "svg" || n.Data == "img" || n.Data == "video" || n.Data == "audio" || n.Data == "figure" || n.Data == "picture") {
		return "" // Игнорируем эти теги и их содержимое
	}

	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractedChildText := extractText(c)
		if extractedChildText != "" {
			if sb.Len() > 0 && !strings.HasSuffix(sb.String(), " ") && !strings.HasPrefix(extractedChildText, " ") {
				sb.WriteString(" ")
			}
			sb.WriteString(extractedChildText)
		}
	}
	return sb.String()
}
