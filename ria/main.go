package main

import (
	"fmt"
	"net/http"
	"strings" // Добавлен для строковых операций

	// "math/rand" // Для примера из вашего кода, здесь не используется
	// "time"      // Для примера из вашего кода, здесь не используется

	"golang.org/x/net/html"
)

// Data определяет структуру для хранения данных о продукте.
type Data struct {
	Title string
	Body  string
}

// Цветовые константы ANSI
const (
	colorReset = "\033[0m"
	colorGreen = "\033[32m"
	colorRed   = "\033[31m"
	// colorYellow = "\033[33m" // Можно добавить желтый для предупреждений
)

func main() {
	// Запускаем парсинг и получаем данные
	allData := parsing_links()
	// Можно добавить здесь использование allData, если необходимо
	if allData == nil || len(allData) == 0 {
		fmt.Println("В итоге не было собрано никаких данных.")
	}
	// fmt.Printf("Всего собрано %d статей.\n", len(allData)) // Пример использования
}

// get_html загружает HTML-страницу по указанному URL и парсит ее
func get_html(pageUrl string) (*html.Node, error) {
	// Выполняет HTTP GET запрос по указанному URL
	resp, err := http.Get(pageUrl)
	if err != nil {
		return nil, err
	}

	// defer гарантирует, что тело ответа будет закрыто перед выходом из функции
	defer resp.Body.Close()

	// Проверяет, успешен ли HTTP-статус ответа (код 200 OK)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка при получении страницы %s: %s", pageUrl, resp.Status)
	}

	// Парсит (разбирает) тело ответа (HTML-документ) в дерево узлов
	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга HTML со страницы %s: %w", pageUrl, err)
	}

	return doc, nil
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

// parsing_links получает ссылки на статьи с ленты новостей
func parsing_links() []Data { // Изменена сигнатура для возврата результата
	// URL ленты новостей
	URL := "https://ria.ru/lenta/"

	// Получаем HTML-документ ленты новостей
	doc, err := get_html(URL)
	if err != nil {
		fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", URL, err)
		return nil // Возвращаем nil в случае ошибки
	}

	// Срез для хранения найденных ссылок
	var foundLinks []string

	// Рекурсивная функция обхода HTML
	var extractLinks func(*html.Node)
	extractLinks = func(h *html.Node) {
		// Проверяем, является ли узел элементом <a>
		if h.Type == html.ElementNode && h.Data == "a" {
			var hrefValue string
			var classAttrValue string
			// Ищем атрибуты "href" и "class"
			for _, attr := range h.Attr {
				if attr.Key == "href" {
					hrefValue = attr.Val
				}
				if attr.Key == "class" {
					classAttrValue = attr.Val
				}
			}

			// Проверяем, содержит ли класс искомую часть
			// Класс на РИА может быть "list-item__title" или "list-item__title color-font-hover-only"
			if strings.Contains(classAttrValue, "list-item__title") {
				if hrefValue != "" {
					// Преобразуем относительные URL в абсолютные
					if strings.HasPrefix(hrefValue, "/") {
						if !strings.HasPrefix(hrefValue, "//") { // для ссылок типа //domain.com/path
							hrefValue = "https://ria.ru" + hrefValue
						} else {
							hrefValue = "https:" + hrefValue // для ссылок типа //ria.ru/something
						}
					}
					// Добавляем ссылку, если она еще не была добавлена
					isDuplicate := false
					for _, l := range foundLinks {
						if l == hrefValue {
							isDuplicate = true
							break
						}
					}
					// Убедимся, что это ссылка на РИА
					if !isDuplicate && strings.HasPrefix(hrefValue, "https://ria.ru") {
						foundLinks = append(foundLinks, hrefValue)
					}
				}
			}
		}

		// Рекурсивно обходим всех потомков текущего узла
		for c := h.FirstChild; c != nil; c = c.NextSibling {
			extractLinks(c)
		}
	}

	// Запускаем обход HTML-дерева
	extractLinks(doc)

	// Выводим информацию о найденных ссылках
	if len(foundLinks) > 0 {
		fmt.Printf("\nНайдено %d уникальных ссылок на статьи:\n", len(foundLinks))
		// for _, link := range foundLinks { // Можно раскомментировать для отладки
		// 	fmt.Println(link)
		// }
	} else {
		fmt.Printf("\nНе найдено ссылок с классом, содержащим 'list-item__title', на странице %s.\n", URL)
		fmt.Println("Возможные причины:")
		fmt.Println("1. HTML-структура сайта изменилась.")
		fmt.Println("2. Ошибка в URL или временные проблемы с доступом к сайту.")
		fmt.Println("Рекомендуется проверить актуальную HTML-структуру страницы.")
	}

	return parsing_page(foundLinks)
}

// parsing_page получает заголовок и текст статьи
func parsing_page(links []string) []Data {
	var products []Data
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Println("Нет ссылок для парсинга.")
		return products
	}

	progressBarLength := 40
	statusTextWidth := 80

	for i, url := range links {
		var title, body string
		var pageStatusMessage string
		var statusMessageColor = colorReset

		doc, err := get_html(url)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", limitString(err.Error(), 50))
			statusMessageColor = colorRed // Ошибка - красный цвет
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
							title = strings.TrimSpace(extractText(h))
						}
					} else if classAttrValue == "article__text" {
						currentTextPart := strings.TrimSpace(extractText(h))
						if currentTextPart != "" {
							if body != "" {
								body += "\n"
							}
							body += currentTextPart
						}
					} else if strings.Contains(classAttrValue, "article__quote-text") {
						quoteTextPart := strings.TrimSpace(extractText(h))
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
	fmt.Printf("\rПарсинг завершен. Собрано %d статей.\n", len(products))

	fmt.Println("\n--- Собранные данные ---")
	if len(products) == 0 {
		fmt.Println("Не удалось собрать данные ни с одной страницы.")
	}
	for idx, product := range products {
		fmt.Printf("\nСтатья #%d\n", idx+1)
		fmt.Printf("Заголовок: %s\n", product.Title)
		fmt.Printf("Тело:\n%s\n", product.Body)
		fmt.Println(strings.Repeat("-", 100))
	}

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
