package parsers

import (
	"fmt"
	"math"
	. "parsing_media/utils"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Константы (Цветовые константы ANSI)
const (
	maxRetries          = 3 // Максимальное количество основных попыток (не считая финальной после минутной паузы)
	initialRetryDelay   = 20 * time.Second
	secondaryRetryDelay = 5 * time.Second
	finalBigDelay       = 1 * time.Minute

	quantityLinksGazeta = 100
	gazetaURL           = "https://www.gazeta.ru"
	gazetaURLNews       = "https://www.gazeta.ru/news/"
	gazetaURLNewPage    = "https://www.gazeta.ru/news/?p=main&d=%d&page=%d"
)

func GazetaMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", ColorYellow, ColorReset)
	_ = parsingLinksGazeta()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func parsingLinksGazeta() []Data {
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

		if h.Type == html.ElementNode && h.Data == "a" && HasAllClasses(h, "b_ear m_techlisting") {
			if len(foundLinks) < quantityLinksGazeta {
				if href, ok := GetAttribute(h, "href"); ok {
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

				if dateStr, ok := GetAttribute(h, "data-pubtime"); ok {
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
		fmt.Printf("\n%s[CRITICAL] Не удалось загрузить стартовую страницу %s после всех попыток: %v. Прерывание.%s\n", ColorRed, gazetaURLNews, err, ColorReset)
		return nil // Критическая ошибка, если даже стартовая страница не грузится
	}
	extractLinks(doc)

	progressBarLength := 40

	for numPage := 2; len(foundLinks) < quantityLinksGazeta; numPage++ {
		// Создание ссылки на новую станицу
		newURL := fmt.Sprintf(gazetaURLNewPage, lastDate, numPage) // Используем исправленную константу и %d

		// Парсинг доп ссылки с повторными попытками
		pageDoc, err := getHTMLWithRetries(newURL) // Используем новую функцию
		if err != nil {
			// Если getHTMLWithRetries вернула ошибку, это означает, что все попытки провалились,
			// и мы решили прервать цикл согласно твоей логике.
			fmt.Printf("\n%s[CRITICAL] Не удалось загрузить страницу %s после всех попыток. Прерывание парсинга дополнительных страниц.%s\n", ColorRed, newURL, ColorReset)
			break // Прерываем цикл for numPage
		}

		extractLinks(pageDoc)

		// Расчет процента выполнения для прогресс-бара
		percent := int((float64(len(foundLinks)) / float64(quantityLinksGazeta)) * 100)
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
		countStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinksGazeta)

		// Выводим прогресс-бар, процент выполнения и статусное сообщение
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, ColorGreen, countStr, ColorReset)

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

	return parsingPageGazeta(foundLinks)
}

func parsingPageGazeta(links []string) []Data {
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
		var statusMessageColor = ColorReset

		doc, err := GetHTML(url)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			statusMessageColor = ColorRed // Ошибка - красный цвет
		} else {
			var tempTitle string
			var accumulatedBodyParts []string // Будем собирать отфильтрованные части текста сюда
			var findData func(*html.Node)
			findData = func(h *html.Node) {
				if h.Type == html.ElementNode {
					// Поиск заголовка (только один раз)
					if tempTitle == "" && HasAllClasses(h, "headline") {
						tempTitle = strings.TrimSpace(ExtractText(h))
						// Не выходим, так как могут быть другие элементы на том же уровне или дальше
					}

					// Поиск и обработка основного текста статьи
					if HasAllClasses(h, "b_article-text") {
						var currentBlockParagraphs []string
						for c := h.FirstChild; c != nil; c = c.NextSibling {
							var paragraphText string
							// Извлекаем текст из тегов <p> или непосредственных текстовых узлов
							if c.Type == html.ElementNode && c.Data == "p" {
								paragraphText = strings.TrimSpace(ExtractText(c))
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
				pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 50))
				statusMessageColor = ColorGreen // Успех - зеленый
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", LimitString(url, 50))
				statusMessageColor = ColorRed // Нет данных - красный
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

		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, statusMessageColor, fullStatusText, ColorReset)
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
	doc, err := GetHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех с первой попытки
	}
	lastErr = err
	fmt.Printf("\n%s[WARN] Первая попытка загрузки %s не удалась: %v. Ждем %s...%s\n", ColorYellow, pageUrl, err, initialRetryDelay, ColorReset)
	time.Sleep(initialRetryDelay)

	// Вторая попытка
	doc, err = GetHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех со второй попытки
	}
	lastErr = err
	fmt.Printf("\n%s[WARN] Вторая попытка загрузки %s не удалась: %v. Ждем %s...%s\n", ColorYellow, pageUrl, err, secondaryRetryDelay, ColorReset)
	time.Sleep(secondaryRetryDelay)

	// Третья попытка (перед минутной паузой)
	doc, err = GetHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех с третьей попытки
	}
	lastErr = err
	fmt.Printf("\n%s[WARN] Третья попытка загрузки %s не удалась: %v. Ждем %s перед финальной попыткой...%s\n", ColorYellow, pageUrl, err, finalBigDelay, ColorReset)
	time.Sleep(finalBigDelay)

	// Финальная попытка после большой паузы
	doc, err = GetHTML(pageUrl)
	if err == nil {
		return doc, nil // Успех с финальной попытки
	}
	lastErr = err // Сохраняем последнюю ошибку
	fmt.Printf("\n%s[ERROR] Финальная попытка загрузки %s также не удалась: %v%s\n", ColorRed, pageUrl, err, ColorReset)
	return nil, fmt.Errorf("все попытки загрузить %s провалились, последняя ошибка: %w", pageUrl, lastErr)
}
