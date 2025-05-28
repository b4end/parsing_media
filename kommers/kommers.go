package kommers

import (
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// Константы (Цветовые константы ANSI)
const (
	quantityLinks     = 100
	kommersURL        = "https://www.kommersant.ru/"
	kommersURLNews    = "https://www.kommersant.ru/lenta"
	kommersURLNewPage = "https://www.kommersant.ru/lenta?page=%d"
)

func KommersMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", ColorYellow, ColorReset)
	_ = parsingLinks()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
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
	doc, err := GetHTML(kommersURL)
	if err != nil {
		fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", kommersURL, err)
	}

	extractLinks(doc)
	progressBarLength := 40

	// Цикл для загрузки ссылок из дополнительных страниц
	for pageNumber := 1; len(found_links) < quantityLinks; pageNumber++ {

		// Парсинг доп ссылки
		doc, err := GetHTML("https://www.kommersant.ru/lenta?page=" + fmt.Sprint(pageNumber))
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
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, ColorGreen, countStr, ColorReset)
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
		var statusMessageColor = ColorReset

		doc, err := GetHTML(URL)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			statusMessageColor = ColorRed // Ошибка - красный цвет
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
							title = strings.TrimSpace(ExtractText(h))
						}
					} else if classValue == "doc__text" {
						currentTextPart := strings.TrimSpace(ExtractText(h))
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
				pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 50))
				statusMessageColor = ColorGreen // Успех - зеленый
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", LimitString(URL, 50))
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
