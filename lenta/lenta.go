package lenta

import (
	"fmt"      // Текст в консоль;
	"net/http" // Выполнение HTTP-запросов;
	"strings"  // Работа со строками;
	"time"

	"golang.org/x/net/html" // Специальная библиотека, для парсинга HTML.
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"

	baseLinksNumber = 100 // Сколько ссылок нужно получить;
)

type Data struct { // Определение структуры того, как будут храниться данные:
	Title string // Сначала заголовок;
	Body  string // Потом остальной текст.
}

func LentaMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", colorYellow, colorReset)
	_ = getLinks()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", colorYellow, formatDuration(totalElapsedTime), colorReset)
}

func getHTML(pageURL string) (*html.Node, error) { // «получитьHTML»: получает HTML-код указанного сайта.
	resp, err := http.Get(pageURL)

	if err != nil { // Если есть какая-либо ошибка, то ...
		return nil, err // ... вывести её.
	}

	if resp.StatusCode != http.StatusOK { // Проверяет, равен ли HTTP-статус ответа коду 200 OK: успешный запрос.
		return nil, fmt.Errorf("ошибка %s при получении страницы %s", resp.Status, pageURL) // Если не успешно, то выдаёт ошибку.
	}

	doc, err := html.Parse(resp.Body) // «Расчленяет» <body>.
	if err != nil {                   // Ошибка, если <body> по какой-то причине не получилось «расчленить».
		return nil, fmt.Errorf("ошибка парсинга HTML со страницы %s: %w", pageURL, err)
	}

	defer resp.Body.Close() // Закрыть <body> после того, как будет получено всё что нужно. Нужно что бы не нагружать ОЗУ.
	return doc, nil
}

func getLinks() []Data { // «получитьСсылки»: получает ссылки с веб-страницы.
	URL := "https://lenta.ru/parts/news/" // Веб-страница, с которой нужно получать ссылки.
	var found_links []string              // Срез (массив) для хранения найденных ссылок.

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

			if classValue == "card-full-news _parts-news" { // Проверяем, соответствует ли класс искомому.
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
					found_links = append(found_links, fmt.Sprint("https://lenta.ru"+hrefValue))
				}
			}
		}

		if len(found_links) < baseLinksNumber {
			for c := h.FirstChild; c != nil; c = c.NextSibling { // Рекурсивно обходим всех потомков текущего узла.
				extractLinks(c)
			}
		}
	}

	fmt.Println("\nНачало парсинга ссылок...")

	// Получаем HTML-документ ленты новостей
	doc, err := getHTML(URL)
	if err != nil {
		fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", URL, err)
	}

	extractLinks(doc)
	progressBarLength := 40

	// Цикл для загрузки ссылок из дополнительных страниц
	for pageNumber := 1; len(found_links) < baseLinksNumber; pageNumber++ {

		// Парсинг доп ссылки
		doc, err := getHTML("https://lenta.ru/parts/news/" + fmt.Sprint(pageNumber))
		if err != nil {
			fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", URL, err)
		}

		extractLinks(doc)

		// Расчет процента выполнения для прогресс-бара
		percent := int((float64(len(found_links)) / float64(baseLinksNumber)) * 100)
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
		countStr := fmt.Sprintf("(%d/%d) ", len(found_links), baseLinksNumber)

		// Выводим прогресс-бар, процент выполнения и статусное сообщение
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, colorGreen, countStr, colorReset)
	}

	// Выводим количество ссылок
	if len(found_links) > 0 {
		fmt.Printf("\n\nНайдено %d уникальных ссылок на статьи\n", len(found_links))
	} else {
		fmt.Printf("\nНе найдено ссылок с классом '%s' на странице %s.\n", "card-full-news _parts-news", URL)
	}

	return getPage(found_links)
}

func getPage(links []string) []Data { // «получитьСтраницу»: получает заголовок и текст.
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

					if classValue == "topic-body__title" {
						if title == "" {
							title = strings.TrimSpace(extractText(h))
						}
					} else if classValue == "topic-body__content-text" {
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
