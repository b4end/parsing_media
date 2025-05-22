package fontanka

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

	quantityLinks      = 100
	fontankaURL        = "https://www.fontanka.ru"
	fontankaURLNews    = "https://www.fontanka.ru/politic/"
	fontankaURLNewPage = "https://www.fontanka.ru/politic/page-%d/"
)

func FontankaMain() {
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
		if h.Type == html.ElementNode && h.Data == "a" && hasAllClasses(h, "header_RL97A") { // Класс для ссылок на статьи
			if len(foundLinks) < quantityLinks {
				if href, ok := getAttribute(h, "href"); ok {
					var fullURL string
					if strings.HasPrefix(href, fontankaURL) {
						fullURL = href
					} else if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
						fullURL = fontankaURL + href
					}

					if fullURL != "" && !seenLinks[fullURL] {
						seenLinks[fullURL] = true
						foundLinks = append(foundLinks, fullURL)
						// fmt.Printf("Найдена ссылка: %s\n", fullURL) // Для отладки
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

	fmt.Printf("\n%s[INFO] Начало сбора ссылок на статьи...%s\n", colorYellow, colorReset)
	doc, err := getHTML(fontankaURLNews)
	if err != nil {
		fmt.Printf("\n%s[CRITICAL] Не удалось загрузить стартовую страницу %s для первоначального сбора ссылок. Ошибка: %s. Завершение работы.%s\n",
			colorRed, fontankaURLNews, err, colorReset)
		return nil
	}
	extractLinks(doc)

	progressBarLength := 40

	// Сбор ссылок с дополнительных страниц, если нужно
	for newPage := 2; len(foundLinks) < quantityLinks; newPage++ { // Начинаем с page-2, т.к. page-0 и page-1 часто дублируют основную
		nowURL := fmt.Sprintf(fontankaURLNewPage, newPage)
		doc, err := getHTML(nowURL)
		if err != nil {
			fmt.Printf("\n%s[WARNING] Не удалось получить страницу %s. Ошибка: %s. Пропуск этой страницы.%s\n",
				colorYellow, nowURL, err, colorReset)
			// Можно решить, прерывать ли цикл или просто пропустить страницу
			// break // если хотим прервать при первой ошибке
			if newPage > 50 { // Ограничение, чтобы не уйти в бесконечный цикл, если что-то не так с сайтом
				fmt.Printf("\n%s[WARNING] Достигнут лимит страниц для поиска ссылок.%s\n", colorYellow, colorReset)
				break
			}
			continue // если хотим пропустить и попробовать следующую
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
		if len(foundLinks) >= quantityLinks {
			break
		}
	}
	fmt.Println() // Перевод строки после прогресс-бара

	if len(foundLinks) > 0 {
		fmt.Printf("\n%s[INFO] Собрано %d уникальных ссылок на статьи.%s\n", colorGreen, len(foundLinks), colorReset)
	} else {
		fmt.Printf("\n%s[WARNING] Не найдено ссылок для парсинга.%s\n", colorYellow, colorReset)
	}
	return parsingPage(foundLinks)
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
	statusTextWidth := 80 // Ширина для текста статуса (включая счетчик)

	for i, url := range links {
		var title string
		var pageStatusMessage string
		var statusMessageColor = colorReset
		var bodyBuilder strings.Builder // Общий bodyBuilder для накопления всех частей тела для ТЕКУЩЕЙ статьи

		doc, err := getHTML(url)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", limitString(err.Error(), statusTextWidth-10)) // Оставляем место для счетчика
			statusMessageColor = colorRed
		} else {
			// Рекурсивная функция для поиска заголовка и ТЕКСТОВЫХ БЛОКОВ статьи
			var extractDataRec func(*html.Node)
			extractDataRec = func(n *html.Node) {
				if n.Type == html.ElementNode {
					// Поиск заголовка
					if title == "" && n.Data == "h1" {
						if classVal, ok := getAttribute(n, "class"); ok && strings.Contains(classVal, "title_BgFsr") { // Убрал точное совпадение, сделал contains
							title = strings.TrimSpace(extractText(n))
						}
					}

					// Поиск и АГРЕГАЦИЯ контейнеров тела статьи
					if n.Data == "div" && hasAllClasses(n, "uiArticleBlockText_5xJo1 text-style-body-1 c-text block_0DdLJ") {
						var currentBlockContentBuilder strings.Builder // Локальный сборщик для ЭТОГО div-блока

						// Вспомогательная функция для сбора текста ИЗ ДЕТЕЙ текущего div-блока (n)
						var collectTextFromCurrentBlock func(*html.Node)
						collectTextFromCurrentBlock = func(contentNode *html.Node) {
							if contentNode.Type == html.ElementNode {
								// Собираем текст из <p> и <a> напрямую
								// Также можно добавить другие теги, если они содержат текст, например, blockquote, li
								if contentNode.Data == "p" || contentNode.Data == "a" || contentNode.Data == "li" || contentNode.Data == "blockquote" {
									partText := strings.TrimSpace(extractText(contentNode))
									if partText != "" {
										if currentBlockContentBuilder.Len() > 0 {
											currentBlockContentBuilder.WriteString("\n\n")
										}
										currentBlockContentBuilder.WriteString(partText)
									}
									// Текст из p/a/li/blockquote собран, дальше в его детей этой функцией не идем,
									// т.к. extractText уже рекурсивно обошел их.
									return
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
							// Рекурсивно обходим детей текущего узла contentNode
							for c := contentNode.FirstChild; c != nil; c = c.NextSibling {
								collectTextFromCurrentBlock(c)
							}
						}

						// Начинаем сбор текста с ДЕТЕЙ текущего div-блока (n)
						for childNode := n.FirstChild; childNode != nil; childNode = childNode.NextSibling {
							collectTextFromCurrentBlock(childNode)
						}

						blockText := currentBlockContentBuilder.String()
						if blockText != "" {
							if bodyBuilder.Len() > 0 {
								bodyBuilder.WriteString("\n\n") // Разделяем содержимое разных div-блоков
							}
							bodyBuilder.WriteString(blockText)
						}
						return // Закончили обработку этого конкретного текстового блока и его содержимого.
					}
				}

				// Продолжаем рекурсию по всем дочерним узлам n, чтобы найти другие части
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					extractDataRec(c)
				}
			}
			extractDataRec(doc) // Запускаем поиск
			currentArticleBody := bodyBuilder.String()

			if title != "" || currentArticleBody != "" {
				articlesData = append(articlesData, Data{Title: title, Body: currentArticleBody})
				pageStatusMessage = fmt.Sprintf("Успех: %s", limitString(title, 50))
				statusMessageColor = colorGreen
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", limitString(url, 50))
				statusMessageColor = colorRed
			}
		}

		percent := int((float64(i+1) / float64(totalLinks)) * 100)
		completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
		// ... (остальная часть прогресс-бара без изменений) ...
		bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
		countStr := fmt.Sprintf("(%d/%d) ", i+1, totalLinks)

		availableWidthForMessage := statusTextWidth - len(countStr) - len("[]") - 5 // 5 для " xxx% "
		if availableWidthForMessage < 10 {
			availableWidthForMessage = 10
		}
		displayMessage := limitString(pageStatusMessage, availableWidthForMessage)

		fullStatusText := countStr + displayMessage
		// Выравнивание пробелами, если нужно, чтобы строка не "прыгала"
		paddingNeeded := statusTextWidth - len(fullStatusText)
		if paddingNeeded < 0 {
			paddingNeeded = 0
		}

		fmt.Printf("\r[%s] %3d%% %s%s%s%s",
			bar, percent,
			statusMessageColor, fullStatusText, strings.Repeat(" ", paddingNeeded), colorReset)
	}
	fmt.Println() // Перевод строки после завершения прогресс-бара

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
		return true // Если целевых классов нет, считаем, что условие выполнено
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
	if length < 3 {
		if length <= 0 {
			return ""
		}
		return s[:length]
	}
	return s[:length-3] + "..."
}

func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		// Убираем лишние пробелы и все переносы строк из текстовых узлов
		return strings.Join(strings.Fields(n.Data), " ")
	}
	if n.Type == html.ElementNode &&
		(n.Data == "script" || n.Data == "style" || n.Data == "noscript" ||
			n.Data == "iframe" || n.Data == "svg" || n.Data == "img" || n.Data == "video" ||
			n.Data == "audio" || n.Data == "figure" || n.Data == "picture") {
		return "" // Игнорируем эти теги и их содержимое
	}

	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractedChildText := extractText(c)
		if extractedChildText != "" {
			// Добавляем пробел между словами из разных TextNode или блочных элементов,
			// если он не был добавлен Fields/Join
			if sb.Len() > 0 {
				lastCharOfSb := sb.String()[sb.Len()-1]
				firstCharOfChild := extractedChildText[0]
				if lastCharOfSb != ' ' && lastCharOfSb != '\n' && firstCharOfChild != ' ' && firstCharOfChild != '\n' {
					sb.WriteString(" ")
				}
			}
			sb.WriteString(extractedChildText)
		}
	}
	return strings.TrimSpace(sb.String()) // Дополнительная очистка пробелов по краям всего собранного текста
}
