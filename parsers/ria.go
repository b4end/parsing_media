package parsers

import (
	"fmt"
	. "parsing_media/utils"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Константы (Цветовые константы ANSI)
const (
	quantityLinksRia = 100
)

// timeRegex находит время в формате ЧЧ:ММ (например, 22:58, 09:30)
var timeRegex = regexp.MustCompile(`(\d{2}:\d{2})`)

func RiaMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", ColorYellow, ColorReset)
	_ = parsingLinksRia()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func parsingLinksRia() []Data {
	// URL ленты новостей
	ria_url := "https://ria.ru/lenta/"

	// Срез для хранения найденных ссылок
	var found_links []string
	// Переменная для хранения времени публикации статей
	var found_time string
	var itr int16 = 0

	// Рекурсивная функция обхода HTML для ссылок
	var extractLinks func(*html.Node)
	extractLinks = func(h *html.Node) {
		// Проверяем, является ли узел элементом <a>
		if h.Type == html.ElementNode && h.Data == "a" {
			var hrefValue string
			var classAttrValue string
			hasCorrectClass := false
			// Ищем атрибуты "href" и "class"
			for _, attr := range h.Attr {
				if attr.Key == "href" {
					hrefValue = attr.Val
				}
				if attr.Key == "class" {
					classAttrValue = attr.Val
				}
			}

			// Проверяем, соответствует ли класс искомому
			if classAttrValue == "list-item__title color-font-hover-only" {
				hasCorrectClass = true
			}

			// Если класс совпадает и есть атрибут href, добавляем ссылку
			if hasCorrectClass && hrefValue != "" {
				// Добавляем ссылку, если она еще не была добавлена
				isDuplicate := false
				for _, l := range found_links {
					if l == hrefValue {
						isDuplicate = true
						break
					}
				}
				// Убедимся, что это ссылка на РИА
				if !isDuplicate && strings.HasPrefix(hrefValue, "https://ria.ru") {
					found_links = append(found_links, hrefValue)
				}
			}
		}
		if len(found_links) < quantityLinksRia {
			// Рекурсивно обходим всех потомков текущего узла
			for c := h.FirstChild; c != nil; c = c.NextSibling {
				extractLinks(c)
			}
		}
	}

	// Рекурсивная функция получения времени публикации последней статьи
	var extractTime func(*html.Node)
	extractTime = func(h *html.Node) {
		itr += 1
		if h.Type == html.ElementNode && h.Data == "div" {
			hasClass := false
			hasDataTypeDate := false
			// Проверка класса и data-type
			for _, attr := range h.Attr {
				if attr.Key == "class" && attr.Val == "list-item__info-item" {
					hasClass = true
				}
				if attr.Key == "data-type" && attr.Val == "date" {
					hasDataTypeDate = true
				}
			}

			// Если класс и data-type совпадает
			if hasClass && hasDataTypeDate {
				// Обходим дочерние узлы, чтобы найти TextNode
				var textContent string
				var findText func(*html.Node)
				findText = func(node *html.Node) {
					if node.Type == html.TextNode {
						textContent += strings.TrimSpace(node.Data)
					}
					for child := node.FirstChild; child != nil; child = child.NextSibling {
						findText(child)
					}
				}
				// Запускаем поиск текста, начиная с найденного div
				findText(h)

				if textContent != "" {
					found_time, _ = extractHHMM(textContent)
				}
			}
		}

		// Рекурсивно обходим всех потомков текущего узла
		for c := h.FirstChild; c != nil; c = c.NextSibling {
			extractTime(c)
		}

		itr = 0
	}

	fmt.Println("\nНачало парсинга ссылок...")

	// Получаем HTML-документ ленты новостей
	doc, err := GetHTML(ria_url)
	if err != nil {
		fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", ria_url, err)
	}

	// Первичное приминение функций
	extractLinks(doc)
	extractTime(doc)

	progressBarLength := 40

	// Цикл для загрузки ссылок из дополнительных страниц
	for true {
		// Проверка на количество ссылок
		if len(found_links) >= quantityLinksRia {
			break
		}

		// Парсинг доп ссылки
		doc, err := GetHTML("https://ria.ru/services/lenta/more.html?id=" + found_links[len(found_links)-1][(len(found_links[len(found_links)-1])-15):(len(found_links[len(found_links)-1])-5)] + "&date=" + found_links[len(found_links)-1][15:23] + "T" + found_time + "59&onedayonly=1&articlemask=lenta_common")
		if err != nil {
			fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", ria_url, err)
		}

		extractLinks(doc)
		extractTime(doc)

		// Расчет процента выполнения для прогресс-бара
		percent := int((float64(len(found_links)) / float64(quantityLinksRia)) * 100)
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
		countStr := fmt.Sprintf("(%d/%d) ", len(found_links), quantityLinksRia)

		// Выводим прогресс-бар, процент выполнения и статусное сообщение
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, ColorGreen, countStr, ColorReset)

	}

	// Выводим количество ссылок
	if len(found_links) > 0 {
		fmt.Printf("\n\nНайдено %d уникальных ссылок на статьи\n", len(found_links))
		//for i, link := range found_links {
		//	fmt.Printf("%d: %s\n", i+1, link)
		//}
	} else {
		fmt.Printf("\nНе найдено ссылок с классом '%s' на странице %s.\n", "list-item__title color-font-hover-only", ria_url)
		fmt.Println("Возможные причины:")
		fmt.Println("1. HTML-структура сайта изменилась.")
		fmt.Println("2. Элементы загружаются динамически с помощью JavaScript.")
		fmt.Println("Рекомендуется проверить актуальную HTML-структуру страницы в браузере.")
	}

	return parsingPageRia(found_links)
}

// parsing_page получает заголовок и текст статьи
func parsingPageRia(links []string) []Data {
	var products []Data
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Println("Нет ссылок для парсинга.")
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
			var get_data func(*html.Node)
			get_data = func(h *html.Node) {
				if h.Type == html.ElementNode {
					var classAttrValue string
					for _, attr := range h.Attr {
						if attr.Key == "class" {
							classAttrValue = attr.Val
							break
						}
					}

					if classAttrValue == "article__title" {
						if title == "" {
							title = strings.TrimSpace(ExtractText(h))
						}
					} else if classAttrValue == "article__text" {
						currentTextPart := strings.TrimSpace(ExtractText(h))
						if currentTextPart != "" {
							if body != "" {
								body += "\n"
							}
							body += currentTextPart
						}
					} else if strings.Contains(classAttrValue, "article__quote-text") {
						quoteTextPart := strings.TrimSpace(ExtractText(h))
						if quoteTextPart != "" {
							if body != "" {
								body += "\n"
							}
							body += quoteTextPart
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

// extractHHMM извлекает "ЧЧ:ММ" и возвращает "ЧЧММ".
func extractHHMM(input string) (string, error) {
	matches := timeRegex.FindStringSubmatch(input)

	if len(matches) > 1 {
		timeWithColon := matches[1] // Это будет "ЧЧ:ММ"
		hhmm := strings.ReplaceAll(timeWithColon, ":", "")
		return hhmm, nil
	}

	return "", fmt.Errorf("время в формате ЧЧ:ММ не найдено в строке: '%s'", input)
}
