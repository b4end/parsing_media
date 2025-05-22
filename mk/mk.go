package mk

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

	quantityLinks = 100
	mkURL         = "https://www.mk.ru"
	mkURLByDate   = "https://www.mk.ru/news/%d/%d/%d/"
)

func MKMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", colorYellow, colorReset)
	_ = parsingLinks()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", colorYellow, formatDuration(totalElapsedTime), colorReset)
}

func parsingLinks() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	var extractLinks func(*html.Node)
	extractLinks = func(h *html.Node) {
		if h == nil {
			return
		}
		if h.Type == html.ElementNode && h.Data == "a" && hasAllClasses(h, "news-listing__item-link") {
			if len(foundLinks) < quantityLinks {
				if href, ok := getAttribute(h, "href"); ok {
					// Убедимся, что это ссылка на MK
					if strings.HasPrefix(href, mkURL) {
						if href != "" && !seenLinks[href] {
							seenLinks[href] = true
							foundLinks = append(foundLinks, href)
						}
					}
				}
			}
		}
		if len(foundLinks) < quantityLinks {
			for c := h.FirstChild; c != nil; c = c.NextSibling {
				extractLinks(c)
			}
		}
	}

	progressBarLength := 40

	for daysAgo := 0; len(foundLinks) < quantityLinks; daysAgo++ {
		nowURL := generateURLForDate(mkURLByDate, generateURLForPastDate(daysAgo))
		doc, err := getHTML(nowURL)
		if err != nil {
			fmt.Printf("\n%s[CRITICAL] Не удалось получить страницy %s. Ошибка: %s. Прерывание парсинга дополнительных страниц.%s\n", colorRed, nowURL, err, colorReset)
			break
		}

		extractLinks(doc)

		percent := int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
		completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
		if completedChars < 0 {
			completedChars = 0
		} else if completedChars > progressBarLength {
			completedChars = progressBarLength
		}
		bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
		countStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, colorGreen, countStr, colorReset)

	}

	if len(foundLinks) > 0 {
		fmt.Printf("\n\n%s[INFO] Собрано %d уникальных ссылок на статьи.%s\n", colorGreen, len(foundLinks), colorReset)
	} else {
		fmt.Printf("\n%s[WARNING] Не найдено ссылок для парсинга.%s\n", colorYellow, colorReset)
	}
	return nil //parsingPage(foundLinks)
}

func parsingPage(links []string) []Data {
	var articlesData []Data
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("\n%s[INFO] Нет ссылок для парсинга статей.%s\n", colorYellow, colorReset)
		return nil
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей...%s\n", colorYellow, totalLinks, colorReset)

	progressBarLength := 40
	statusTextWidth := 80

	for i, url := range links {
		var title, body string
		var pageStatusMessage string
		var statusMessageColor = colorReset

		doc, err := getHTML(url)
		if err != nil {
			// Формируем сообщение для прогресс-бара
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", limitString(err.Error(), 50))
			statusMessageColor = colorRed
		} else {
			// Рекурсивная функция для поиска заголовка и тела статьи
			var extractData func(*html.Node)
			extractData = func(n *html.Node) {
				// Если и заголовок, и тело уже найдены, дальше не ищем
				if title != "" && body != "" {
					return
				}

				if n.Type == html.ElementNode {
					// Поиск заголовка
					if title == "" && n.Data == "h1" {
						if classVal, ok := getAttribute(n, "class"); ok && classVal == "article__title" {
							title = strings.TrimSpace(extractText(n))
						}
					}

					// Поиск контейнера тела статьи
					if body == "" && n.Data == "div" {
						if classVal, ok := getAttribute(n, "class"); ok && classVal == "article__body" {
							var bodyBuilder strings.Builder
							var collectTextRecursively func(*html.Node)

							collectTextRecursively = func(contentNode *html.Node) {
								if contentNode.Type == html.ElementNode {
									if contentNode.Data == "p" || contentNode.Data == "a" {
										partText := strings.TrimSpace(extractText(contentNode))
										if partText != "" {
											if bodyBuilder.Len() > 0 {
												bodyBuilder.WriteString("\n\n") // Разделяем абзацы/цитаты
											}
											bodyBuilder.WriteString(partText)
										}
										return // Текст из p/blockquote собран, дальше в его детей не идем этой функцией
									}
									// Пропускаем теги, которые точно не содержат текст статьи или могут его дублировать
									if contentNode.Data == "script" || contentNode.Data == "style" ||
										contentNode.Data == "noscript" || contentNode.Data == "iframe" ||
										contentNode.Data == "img" || contentNode.Data == "figure" ||
										contentNode.Data == "picture" || contentNode.Data == "video" ||
										contentNode.Data == "audio" {
										return
									}
								}
								// Рекурсивно обходим детей текущего узла
								for c := contentNode.FirstChild; c != nil; c = c.NextSibling {
									collectTextRecursively(c)
								}
							}

							// Начинаем сбор текста с детей контейнера js-mediator-article
							for c := n.FirstChild; c != nil; c = c.NextSibling {
								collectTextRecursively(c)
							}
							body = bodyBuilder.String()
						}
					}
				}

				// Продолжаем рекурсию по дочерним узлам, если что-то еще не найдено
				if title == "" || body == "" {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						extractData(c)
						// Если после обхода ребенка все нашлось, можно прервать обход остальных сиблингов
						if title != "" && body != "" {
							break
						}
					}
				}
			}
			extractData(doc) // Запускаем поиск

			if title != "" || body != "" {
				articlesData = append(articlesData, Data{Title: title, Body: body})
				pageStatusMessage = fmt.Sprintf("Успех: %s", limitString(title, 50))
				statusMessageColor = colorGreen
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", limitString(url, 50))
				statusMessageColor = colorRed
			}
		}

		percent := int((float64(i+1) / float64(totalLinks)) * 100)
		completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
		if completedChars < 0 {
			completedChars = 0
		} else if completedChars > progressBarLength {
			completedChars = progressBarLength
		}

		bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
		countStr := fmt.Sprintf("(%d/%d) ", i+1, totalLinks)

		// Обрезаем pageStatusMessage, если оно слишком длинное для выделенного места
		availableWidthForMessage := statusTextWidth - len(countStr)
		if availableWidthForMessage < 10 { // Минимальная ширина для сообщения
			availableWidthForMessage = 10
		}
		displayMessage := limitString(pageStatusMessage, availableWidthForMessage)

		// Собираем полную строку статуса и выравниваем пробелами, если нужно
		fullStatusText := countStr + displayMessage
		if len(fullStatusText) < statusTextWidth {
			fullStatusText += strings.Repeat(" ", statusTextWidth-len(fullStatusText))
		} else if len(fullStatusText) > statusTextWidth { // На всякий случай, если limitString не идеально сработал
			fullStatusText = fullStatusText[:statusTextWidth]
		}

		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, statusMessageColor, fullStatusText, colorReset)
	}

	// Перевод строки после завершения прогресс-бара и очистка строки прогресс-бара
	//fmt.Printf("\r%s\r", strings.Repeat(" ", progressBarLength+5+statusTextWidth+len(colorGreen)+len(colorReset))) // +5 для " xxx% "

	if len(articlesData) > 0 {
		fmt.Printf("\n\n%s[INFO] Парсинг статей завершен. Собрано %d статей.%s\n", colorGreen, len(articlesData), colorReset)
		//for idx, product := range articlesData {
		//	fmt.Printf("\nСтатья #%d\n", idx+1)
		//	fmt.Printf("Заголовок: %s\n", product.Title)
		//	fmt.Printf("Тело:\n%s\n", product.Body)
		//	fmt.Println(strings.Repeat("-", 100))
		//}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[WARNING] Парсинг статей завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", colorYellow, totalLinks, colorReset)
	} else {
		// Этот случай уже обработан в начале функции
	}
	return articlesData
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

// generateURLForDate создает URL для новостей на указанную дату
func generateURLForDate(url string, date time.Time) string {
	year := date.Year()
	month := int(date.Month()) // time.Month() возвращает тип time.Month, приводим к int
	day := date.Day()
	return fmt.Sprintf(url, year, month, day)
}

// generateURLForPastDate генерирует URL для даты N дней назад
func generateURLForPastDate(daysAgo int) time.Time {
	today := time.Now()
	pastDate := today.AddDate(0, 0, -daysAgo) // Вычитаем дни
	return pastDate
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
