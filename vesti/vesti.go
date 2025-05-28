package vesti

import (
	"encoding/json"
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// VestiAPINewsItem определяет структуру для одного элемента новости из JSON API
type VestiAPINewsItem struct {
	URL string `json:"url"`
}

// VestiAPIResponse определяет структуру для ответа JSON API
type VestiAPIResponse struct {
	Success    bool `json:"success"`
	Pagination struct {
		Next string `json:"next"`
	} `json:"pagination"`
	Data []VestiAPINewsItem `json:"data"`
}

// Константы (Цветовые константы ANSI)
const (
	quantityLinks   = 100
	vestiURL        = "https://www.vesti.ru"
	vestiURLNews    = "https://www.vesti.ru/news"
	vestiURLNewPage = "https://www.vesti.ru/api/news?page=%d"
)

func VestiMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", ColorYellow, ColorReset)
	_ = parsingLinks()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func parsingLinks() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	var extractLinks func(*html.Node)
	extractLinks = func(h *html.Node) {
		if h == nil {
			return
		}
		if h.Type == html.ElementNode && h.Data == "a" && HasAllClasses(h, "list__pic-wrapper") {
			if len(foundLinks) < quantityLinks {
				if href, ok := GetAttribute(h, "href"); ok {
					fullHref := ""
					if strings.HasPrefix(href, "/") {
						fullHref = vestiURL + href
					} else if strings.HasPrefix(href, vestiURL) {
						fullHref = href
					}
					if fullHref != "" && !seenLinks[fullHref] {
						seenLinks[fullHref] = true
						foundLinks = append(foundLinks, fullHref)
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

	fmt.Printf("\n%s[INFO] Начало сбора ссылок на статьи...%s\n", ColorYellow, ColorReset)
	doc, err := GetHTML(vestiURLNews)
	if err != nil {
		fmt.Printf("\n%s[CRITICAL] Не удалось загрузить стартовую страницу %s для первоначального сбора ссылок. Ошибка: %s. Завершение работы.%s\n", ColorRed, vestiURLNews, err, ColorReset)
		return nil
	}
	extractLinks(doc)

	progressBarLength := 40
	for numPage := 2; len(foundLinks) < quantityLinks; numPage++ {
		newURL := fmt.Sprintf(vestiURLNewPage, numPage)
		jsonData, err := GetJSON(newURL)
		if err != nil {
			fmt.Printf("\n%s[CRITICAL] Не удалось получить JSON-данные со страницы %s. Ошибка: %s. Прерывание парсинга дополнительных страниц.%s\n", ColorRed, newURL, err, ColorReset)
			break
		}

		var apiResponse VestiAPIResponse
		jsonBytes, err := json.Marshal(jsonData)
		if err != nil {
			fmt.Printf("\n%s[ERROR] Не удалось пере-маршализовать JSON для страницы %s. Ошибка: %s. Пропуск этой страницы.%s\n", ColorRed, newURL, err, ColorReset)
			continue
		}
		err = json.Unmarshal(jsonBytes, &apiResponse)
		if err != nil {
			fmt.Printf("\n%s[CRITICAL] Не удалось преобразовать JSON в структуру VestiAPIResponse для страницы %s. Ошибка: %s. Прерывание парсинга дополнительных страниц.%s\n", ColorRed, newURL, err, ColorReset)
			break
		}

		if !apiResponse.Success {
			fmt.Printf("\n%s[WARNING] API для страницы %s сообщило об ошибке (success: false). Прерывание парсинга дополнительных страниц.%s\n", ColorYellow, newURL, ColorReset)
			break
		}
		if len(apiResponse.Data) == 0 {
			fmt.Printf("\n%s[INFO] На странице %s (JSON) больше нет новостей. Завершение сбора ссылок (достигнут конец данных).%s\n", ColorYellow, newURL, ColorReset)
			break
		}

		for _, item := range apiResponse.Data {
			if len(foundLinks) >= quantityLinks {
				break
			}
			if item.URL == "" {
				continue
			}
			fullHref := ""
			if strings.HasPrefix(item.URL, "/") {
				fullHref = vestiURL + item.URL
			} else if strings.HasPrefix(item.URL, vestiURL) {
				fullHref = item.URL
			} else {
				continue
			}
			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
		percent := int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
		completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
		if completedChars < 0 {
			completedChars = 0
		} else if completedChars > progressBarLength {
			completedChars = progressBarLength
		}
		bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
		countStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, ColorGreen, countStr, ColorReset)
	}

	if len(foundLinks) > 0 {
		fmt.Printf("\n\n%s[INFO] Собрано %d уникальных ссылок на статьи.%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("\n%s[WARNING] Не найдено ссылок для парсинга.%s\n", ColorYellow, ColorReset)
	}
	return parsingPage(foundLinks)
}

func parsingPage(links []string) []Data {
	var articlesData []Data
	totalLinks := len(links)

	if totalLinks == 0 {
		fmt.Printf("\n%s[INFO] Нет ссылок для парсинга статей.%s\n", ColorYellow, ColorReset)
		return nil
	}
	fmt.Printf("\n%s[INFO] Начало парсинга %d статей...%s\n", ColorYellow, totalLinks, ColorReset)

	progressBarLength := 40
	statusTextWidth := 80

	for i, url := range links {
		var title, body string
		var pageStatusMessage string
		var statusMessageColor = ColorReset

		doc, err := GetHTML(url)
		if err != nil {
			// Формируем сообщение для прогресс-бара
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			statusMessageColor = ColorRed
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
						if classVal, ok := GetAttribute(n, "class"); ok && classVal == "article__title" {
							title = strings.TrimSpace(ExtractText(n))
						}
					}

					// Поиск контейнера тела статьи
					if body == "" && n.Data == "div" {
						if classVal, ok := GetAttribute(n, "class"); ok && classVal == "js-mediator-article" {
							var bodyBuilder strings.Builder
							var collectTextRecursively func(*html.Node)

							collectTextRecursively = func(contentNode *html.Node) {
								if contentNode.Type == html.ElementNode {
									// Собираем текст из <p> и <blockquote>
									if contentNode.Data == "p" || contentNode.Data == "blockquote" {
										partText := strings.TrimSpace(ExtractText(contentNode))
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
				pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 50))
				statusMessageColor = ColorGreen
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", LimitString(url, 50))
				statusMessageColor = ColorRed
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
		displayMessage := LimitString(pageStatusMessage, availableWidthForMessage)

		// Собираем полную строку статуса и выравниваем пробелами, если нужно
		fullStatusText := countStr + displayMessage
		if len(fullStatusText) < statusTextWidth {
			fullStatusText += strings.Repeat(" ", statusTextWidth-len(fullStatusText))
		} else if len(fullStatusText) > statusTextWidth { // На всякий случай, если limitString не идеально сработал
			fullStatusText = fullStatusText[:statusTextWidth]
		}

		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, statusMessageColor, fullStatusText, ColorReset)
	}

	// Перевод строки после завершения прогресс-бара и очистка строки прогресс-бара
	//fmt.Printf("\r%s\r", strings.Repeat(" ", progressBarLength+5+statusTextWidth+len(colorGreen)+len(colorReset))) // +5 для " xxx% "

	if len(articlesData) > 0 {
		fmt.Printf("\n\n%s[INFO] Парсинг статей завершен. Собрано %d статей.%s\n", ColorGreen, len(articlesData), ColorReset)
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[WARNING] Парсинг статей завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorYellow, totalLinks, ColorReset)
	} else {
		// Этот случай уже обработан в начале функции
	}
	return articlesData
}
