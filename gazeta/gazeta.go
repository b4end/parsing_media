package gazeta

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Product определяет структуру для хранения данных о продукте
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

	maxRetries          = 3 // Максимальное количество основных попыток (не считая финальной после минутной паузы)
	initialRetryDelay   = 20 * time.Second
	secondaryRetryDelay = 5 * time.Second
	finalBigDelay       = 1 * time.Minute

	quantityLinks    = 100
	gazetaURL        = "https://www.gazeta.ru"
	gazetaURLNews    = "https://www.gazeta.ru/news/"
	gazetaURLNewPage = "https://www.gazeta.ru/news/?p=main&d=%d&page=%d"
)

func GazetaMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", colorYellow, colorReset)
	_ = parsingLinks()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", colorYellow, formatDuration(totalElapsedTime), colorReset)
}

func parsingLinks() []Data {
	// Срез для хранения ссылок (если порядок важен для последующего использования)
	var foundLinks []string
	// Карта для быстрой проверки уникальности. Ключ - ссылка, значение - bool (true, если ссылка уже есть)
	seenLinks := make(map[string]bool)
	// Переменная для хранения наименьшего data-pubtime
	var lastDate int64 = math.MaxInt64

	var extractLinks func(*html.Node)
	// Внутри функции extractLinks
	extractLinks = func(h *html.Node) {
		if h == nil { // Важная проверка
			return
		}

		if h.Type == html.ElementNode && h.Data == "a" && hasAllClasses(h, "b_ear m_techlisting") {
			if len(foundLinks) < quantityLinks {
				if href, ok := getAttribute(h, "href"); ok {
					fullHref := ""
					if strings.HasPrefix(href, "/") {
						fullHref = gazetaURL + href
					} else if strings.HasPrefix(href, gazetaURL) {
						fullHref = href
					}

					if fullHref != "" {
						// Проверяем, есть ли ссылка в карте. Это операция с O(1) средней сложностью.
						if !seenLinks[fullHref] {
							seenLinks[fullHref] = true                // Добавляем ссылку в карту
							foundLinks = append(foundLinks, fullHref) // Добавляем в срез (если нужен упорядоченный список)
							// fmt.Printf("Найдена уникальная ссылка: %s\n", fullHref) // Для отладки
						}
					}
				}

				if dateStr, ok := getAttribute(h, "data-pubtime"); ok {
					// Преобразуем data-modif из строки в int64
					if dataInt, err := strconv.ParseInt(dateStr, 10, 64); err == nil {
						// Обновляем global lastDate на минимальное найденное значение
						if dataInt < lastDate {
							lastDate = dataInt
						}
					}
				}
			}
		}
		// Рекурсивно обходим всех потомков текущего узла
		for c := h.FirstChild; c != nil; c = c.NextSibling {
			extractLinks(c)
		}
	}

	fmt.Println("\nНачало парсинга ссылок...")

	// Получаем HTML-документ ленты новостей с повторными попытками
	doc, err := getHTMLWithRetries(gazetaURLNews) // Используем новую функцию
	if err != nil {
		fmt.Printf("\n%s[CRITICAL] Не удалось загрузить стартовую страницу %s после всех попыток: %v. Прерывание.%s\n", colorRed, gazetaURLNews, err, colorReset)
		return nil // Критическая ошибка, если даже стартовая страница не грузится
	}
	extractLinks(doc)

	progressBarLength := 40

	for numPage := 2; len(foundLinks) < quantityLinks; numPage++ {
		// Создание ссылки на новую станицу
		newURL := fmt.Sprintf(gazetaURLNewPage, lastDate, numPage) // Используем исправленную константу и %d

		// Парсинг доп ссылки с повторными попытками
		pageDoc, err := getHTMLWithRetries(newURL) // Используем новую функцию
		if err != nil {
			// Если getHTMLWithRetries вернула ошибку, это означает, что все попытки провалились,
			// и мы решили прервать цикл согласно твоей логике.
			fmt.Printf("\n%s[CRITICAL] Не удалось загрузить страницу %s после всех попыток. Прерывание парсинга дополнительных страниц.%s\n", colorRed, newURL, colorReset)
			break // Прерываем цикл for numPage
		}

		extractLinks(pageDoc)

		// Расчет процента выполнения для прогресс-бара
		percent := int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
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
		countStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)

		// Выводим прогресс-бар, процент выполнения и статусное сообщение
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, colorGreen, countStr, colorReset)

	}

	// Выводим количество ссылок
	if len(foundLinks) > 0 {
		fmt.Printf("\n\nНайдено %d уникальных ссылок на статьи\n", len(foundLinks))
		//for i, link := range found_links {
		//	fmt.Printf("%d: %s\n", i+1, link)
		//}
	} else {
		fmt.Printf("\nНе найдено ссылок с классом '%s' на странице %s.\n", "b_ear m_techlisting", gazetaURLNews)
	}

	return parsingPage(foundLinks)
}

func parsingPage(links []string) []Data {
	// Массив для хранения данных новостей
	var products []Data
	// Переменная хронящая длинну среза ссылок
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Println("Нет ссылок для парсинга")
		return nil
	} else {
		fmt.Println("\nНачало парсинга статей...")
	}

	progressBarLength := 40
	statusTextWidth := 80

	for i, url := range links {
		var title, body string
		var pageStatusMessage string
		var statusMessageColor = colorReset

		doc, err := getHTML(url)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", limitString(err.Error(), 50))
			statusMessageColor = colorRed // Ошибка - красный цвет
		} else {
			var tempTitle string
			var accumulatedBodyParts []string // Будем собирать отфильтрованные части текста сюда
			var findData func(*html.Node)
			findData = func(h *html.Node) {
				if h.Type == html.ElementNode {
					// Поиск заголовка (только один раз)
					if tempTitle == "" && hasAllClasses(h, "headline") {
						tempTitle = strings.TrimSpace(extractText(h))
						// Не выходим, так как могут быть другие элементы на том же уровне или дальше
					}

					// Поиск и обработка основного текста статьи
					if hasAllClasses(h, "b_article-text") {
						var currentBlockParagraphs []string
						for c := h.FirstChild; c != nil; c = c.NextSibling {
							var paragraphText string
							// Извлекаем текст из тегов <p> или непосредственных текстовых узлов
							if c.Type == html.ElementNode && c.Data == "p" {
								paragraphText = strings.TrimSpace(extractText(c))
							} else if c.Type == html.TextNode {
								trimmedData := strings.TrimSpace(c.Data)
								if trimmedData != "" { // Убедимся, что текстовый узел не пустой после обрезки пробелов
									paragraphText = trimmedData
								}
							}

							if paragraphText != "" {
								// Применяем фильтры
								if strings.Contains(paragraphText, "Что думаешь?") {
									continue // Пропускаем этот параграф
								}
								if strings.HasPrefix(paragraphText, "Ранее ") {
									continue // Пропускаем этот параграф
								}
								currentBlockParagraphs = append(currentBlockParagraphs, paragraphText)
							}
						}
						if len(currentBlockParagraphs) > 0 {
							accumulatedBodyParts = append(accumulatedBodyParts, strings.Join(currentBlockParagraphs, "\n"))
						}
					}
				}
				for c := h.FirstChild; c != nil; c = c.NextSibling {
					findData(c)
				}
			}
			findData(doc)
			title = tempTitle
			body = strings.Join(accumulatedBodyParts, "\n\n")

			// Пост-обработка для удаления заголовка из тела, если он там оказался
			if title != "" && body != "" && strings.Contains(body, title) {
				// Это грубый способ, может удалить лишнее, если заголовок - часть обычного текста.
				// Более точный способ - при extractText для body не включать узлы с классом headline.
				body = strings.Replace(body, title, "", 1) // Удалить первое вхождение
				body = strings.TrimSpace(body)             // Убрать лишние пробелы/переводы строк
			}

			if title != "" || body != "" {
				products = append(products, Data{Title: title, Body: body})
				pageStatusMessage = fmt.Sprintf("Успех: %s", limitString(title, 50))
				statusMessageColor = colorGreen // Успех - зеленый
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", limitString(url, 50))
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

// getHTMLWithRetries пытается загрузить HTML с указанного URL с логикой повторных попыток.
// Возвращает *html.Node и ошибку. Если все попытки неудачны, возвращает последнюю ошибку.
func getHTMLWithRetries(pageUrl string) (*html.Node, error) {
	var lastErr error

	// Первая попытка (основная)
	doc, err := getHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех с первой попытки
	}
	lastErr = err
	fmt.Printf("\n%s[WARN] Первая попытка загрузки %s не удалась: %v. Ждем %s...%s\n", colorYellow, pageUrl, err, initialRetryDelay, colorReset)
	time.Sleep(initialRetryDelay)

	// Вторая попытка
	doc, err = getHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех со второй попытки
	}
	lastErr = err
	fmt.Printf("\n%s[WARN] Вторая попытка загрузки %s не удалась: %v. Ждем %s...%s\n", colorYellow, pageUrl, err, secondaryRetryDelay, colorReset)
	time.Sleep(secondaryRetryDelay)

	// Третья попытка (перед минутной паузой)
	doc, err = getHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех с третьей попытки
	}
	lastErr = err
	fmt.Printf("\n%s[WARN] Третья попытка загрузки %s не удалась: %v. Ждем %s перед финальной попыткой...%s\n", colorYellow, pageUrl, err, finalBigDelay, colorReset)
	time.Sleep(finalBigDelay)

	// Финальная попытка после большой паузы
	doc, err = getHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех с финальной попытки
	}
	lastErr = err // Сохраняем последнюю ошибку
	fmt.Printf("\n%s[ERROR] Финальная попытка загрузки %s также не удалась: %v%s\n", colorRed, pageUrl, err, colorReset)
	return nil, fmt.Errorf("все попытки загрузить %s провалились, последняя ошибка: %w", pageUrl, lastErr)
}

// getHTML (твоя оригинальная функция, возможно с User-Agent и таймаутом)
func getHTML(pageUrl string) (*html.Node, error) {
	client := &http.Client{
		Timeout: 30 * time.Second, // Устанавливаем таймаут на запрос
	}
	req, err := http.NewRequest("GET", pageUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса для %s: %w", pageUrl, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка HTTP GET для %s: %w", pageUrl, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка при получении страницы %s: %s (код %d)", pageUrl, resp.Status, resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга HTML со страницы %s: %w", pageUrl, err)
	}
	return doc, nil
}

// Вспомогательная функция для форматирования time.Duration
func formatDuration(d time.Duration) string {
	// Округляем до ближайшей миллисекунды для более чистого вывода
	d = d.Round(time.Millisecond)

	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		// Формат: X.YYYs (например, 5.123s)
		return fmt.Sprintf("%.3fs", d.Seconds())
	}

	// Извлекаем минуты
	minutes := int64(d.Minutes())
	// Оставшаяся часть после вычета целых минут
	remainingSeconds := d - (time.Duration(minutes) * time.Minute)

	// Форматируем оставшиеся секунды с миллисекундами
	secondsWithMillis := remainingSeconds.Seconds()

	// Собираем строку: Xm Y.ZZZs
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

// hasAllClasses проверяет, содержит ли узел все указанные классы
func hasAllClasses(h *html.Node, targetClasses string) bool {
	if h == nil {
		return false
	}
	classAttr, ok := getAttribute(h, "class")
	if !ok {
		return false
	}

	actualClasses := strings.Fields(classAttr)
	expectedClasses := strings.Fields(targetClasses) // Разбиваем строку targetClasses на отдельные классы

	if len(expectedClasses) == 0 {
		return true // Если не указано целевых классов, считаем, что условие выполнено
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
			return false // Если хотя бы один из ожидаемых классов не найден
		}
	}
	return true // Все ожидаемые классы найдены
}

func limitString(s string, length int) string {
	if len(s) <= length {
		return s
	}
	if length <= 3 {
		return s[:length]
	}
	return s[:length-3] + "..."
}

// Вспомогательная функция для извлечения всего текстового содержимого из узла и его потомков
func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	// Игнорируем содержимое тегов, которые не несут видимого текста
	if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "noscript" || n.Data == "iframe") {
		return ""
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(extractText(c))
	}
	return sb.String()
}
