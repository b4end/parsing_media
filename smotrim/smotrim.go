package smotrim

import (
	"encoding/json"
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"golang.org/x/net/html" // Используется parsingPage, не parsingLinks
)

// SmotrimAPIListItem определяет структуру для одного элемента в списке из JSON API smotrim.ru
type SmotrimAPIListItem struct {
	Type string `json:"type"` // Тип элемента, например, "article"
	Link string `json:"link"` // Относительная ссылка на статью, например, "/article/4514928"
}

// SmotrimAPIMoreLink определяет структуру для ссылки "more" (Показать еще)
type SmotrimAPIMoreLink struct {
	URL string `json:"url"` // Относительный URL для следующей страницы, например, "/api/articles/regionmix?page=3&tags=3515"
}

// SmotrimAPIContentControl определяет структуру для contentControl
type SmotrimAPIContentControl struct {
	More SmotrimAPIMoreLink `json:"more"`
}

// SmotrimAPIContent определяет структуру для одного блока "content" в массиве "contents"
type SmotrimAPIContent struct {
	Alias          string                   `json:"alias"` // Алиас блока, например, "articles"
	List           []SmotrimAPIListItem     `json:"list"`  // Список элементов (статей)
	ContentControl SmotrimAPIContentControl `json:"contentControl"`
}

// SmotrimAPIResponse определяет общую структуру ответа JSON API smotrim.ru
type SmotrimAPIResponse struct {
	Contents []SmotrimAPIContent `json:"contents"` // Массив контентных блоков
}

// Константы (Цветовые константы ANSI)
const (
	quantityLinks     = 100
	smotrimURL        = "https://smotrim.ru"
	smotrimURLNewPage = "https://smotrim.ru/api/search-articles?q=&page=%d&sort=date&date=%s"
)

func SmotrimMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", ColorYellow, ColorReset)
	_ = parsingLinks()

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func parsingLinks() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool) // Карта для отслеживания уникальных ссылок

	fmt.Printf("\n%s[INFO] Начало сбора ссылок на статьи...%s\n", ColorYellow, ColorReset)

	progressBarLength := 40

	// Начальное отображение прогресс-бара
	initialPercent := 0
	// len(foundLinks) здесь 0, поэтому initialPercent будет 0
	if quantityLinks > 0 && len(foundLinks) > 0 {
		initialPercent = int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
	}
	if initialPercent > 100 {
		initialPercent = 100
	} // Ограничение сверху
	initialCompletedChars := int((float64(initialPercent) / 100.0) * float64(progressBarLength))
	if initialCompletedChars < 0 {
		initialCompletedChars = 0
	}
	if initialCompletedChars > progressBarLength {
		initialCompletedChars = progressBarLength
	}
	barInitial := strings.Repeat("█", initialCompletedChars) + strings.Repeat("-", progressBarLength-initialCompletedChars)
	countStrInitial := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)
	fmt.Printf("\r[%s] %3d%% %s%s%s", barInitial, initialPercent, ColorGreen, countStrInitial, ColorReset)

	// Внешний цикл: итерация по датам
	for daysAgo := 0; len(foundLinks) < quantityLinks; daysAgo++ {
		currentDateFormatted := generateURLForDateFormatted(generateURLForPastDate(daysAgo))
		// 1. Формируем БАЗОВЫЙ URL для ТЕКУЩЕЙ ДАТЫ (первая страница API всегда page=1)
		paginatingURL := fmt.Sprintf(smotrimURLNewPage, 1, currentDateFormatted)

		// fmt.Printf("\n%s[DEBUG] Проверка даты: %s%s\n", colorYellow, currentDateFormatted, colorReset) // Для отладки

		// Внутренний цикл: пагинация для ТЕКУЩЕЙ ДАТЫ
		for paginatingURL != "" && len(foundLinks) < quantityLinks {
			jsonData, err := GetJSON(paginatingURL)
			if err != nil {
				fmt.Printf("\n%s[CRITICAL] Не удалось получить JSON с %s. Ошибка: %s. Остановка пагинации для этой даты.%s\n", ColorRed, paginatingURL, err, ColorReset)
				paginatingURL = "" // Прекращаем пагинацию для этой даты
				// Перерисовываем прогресс-бар в текущем состоянии
				currentPercent := 0
				if quantityLinks > 0 {
					currentPercent = int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
				}
				if currentPercent > 100 {
					currentPercent = 100
				}
				currentCompletedChars := int((float64(currentPercent) / 100.0) * float64(progressBarLength))
				if currentCompletedChars < 0 {
					currentCompletedChars = 0
				}
				if currentCompletedChars > progressBarLength {
					currentCompletedChars = progressBarLength
				}
				currentBar := strings.Repeat("█", currentCompletedChars) + strings.Repeat("-", progressBarLength-currentCompletedChars)
				currentCountStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)
				fmt.Printf("\r[%s] %3d%% %s%s%s", currentBar, currentPercent, ColorGreen, currentCountStr, ColorReset)
				continue // Завершит текущую итерацию внутреннего цикла, т.к. paginatingURL пуст
			}

			var apiResponse SmotrimAPIResponse
			jsonBytes, err := json.Marshal(jsonData) // jsonData имеет тип interface{}
			if err != nil {
				fmt.Printf("\n%s[ERROR] Не удалось пере-маршализовать JSON для страницы %s. Ошибка: %s. Остановка пагинации для этой даты.%s\n", ColorRed, paginatingURL, err, ColorReset)
				paginatingURL = ""
				currentPercent := 0
				if quantityLinks > 0 {
					currentPercent = int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
				}
				if currentPercent > 100 {
					currentPercent = 100
				}
				currentCompletedChars := int((float64(currentPercent) / 100.0) * float64(progressBarLength))
				if currentCompletedChars < 0 {
					currentCompletedChars = 0
				}
				if currentCompletedChars > progressBarLength {
					currentCompletedChars = progressBarLength
				}
				currentBar := strings.Repeat("█", currentCompletedChars) + strings.Repeat("-", progressBarLength-currentCompletedChars)
				currentCountStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)
				fmt.Printf("\r[%s] %3d%% %s%s%s", currentBar, currentPercent, ColorGreen, currentCountStr, ColorReset)
				break
			}
			err = json.Unmarshal(jsonBytes, &apiResponse)
			if err != nil {
				fmt.Printf("\n%s[CRITICAL] Не удалось преобразовать JSON в структуру SmotrimAPIResponse для страницы %s. Ошибка: %s. Остановка пагинации для этой даты.%s\n", ColorRed, paginatingURL, err, ColorReset)
				paginatingURL = ""
				currentPercent := 0
				if quantityLinks > 0 {
					currentPercent = int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
				}
				if currentPercent > 100 {
					currentPercent = 100
				}
				currentCompletedChars := int((float64(currentPercent) / 100.0) * float64(progressBarLength))
				if currentCompletedChars < 0 {
					currentCompletedChars = 0
				}
				if currentCompletedChars > progressBarLength {
					currentCompletedChars = progressBarLength
				}
				currentBar := strings.Repeat("█", currentCompletedChars) + strings.Repeat("-", progressBarLength-currentCompletedChars)
				currentCountStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)
				fmt.Printf("\r[%s] %3d%% %s%s%s", currentBar, currentPercent, ColorGreen, currentCountStr, ColorReset)
				break
			}

			if len(apiResponse.Contents) == 0 {
				// fmt.Printf("\n%s[INFO] API для страницы %s вернуло пустой массив 'contents'. Остановка пагинации для этой даты.%s\n", colorYellow, paginatingURL, colorReset)
				paginatingURL = ""
				break
			}

			nextPaginatingURLFromCurrentResponse := ""

			var primaryContentBlock *SmotrimAPIContent
			for i := range apiResponse.Contents {
				if apiResponse.Contents[i].Alias == "articles" {
					primaryContentBlock = &apiResponse.Contents[i]
					break
				}
			}
			if primaryContentBlock == nil && len(apiResponse.Contents) == 1 {
				primaryContentBlock = &apiResponse.Contents[0]
			}

			if primaryContentBlock != nil {
				for _, item := range primaryContentBlock.List {
					if len(foundLinks) >= quantityLinks {
						break
					}
					if item.Link == "" || item.Type != "article" {
						continue
					}
					fullHref := ""
					if strings.HasPrefix(item.Link, "/") {
						fullHref = smotrimURL + item.Link
					} else if strings.HasPrefix(item.Link, smotrimURL) {
						fullHref = item.Link
					} else {
						continue
					}

					if fullHref != "" && !seenLinks[fullHref] {
						seenLinks[fullHref] = true
						foundLinks = append(foundLinks, fullHref)
					}
				}

				if primaryContentBlock.ContentControl.More.URL != "" {
					relativeNextURL := primaryContentBlock.ContentControl.More.URL
					if strings.HasPrefix(relativeNextURL, "/") {
						nextPaginatingURLFromCurrentResponse = smotrimURL + relativeNextURL
					} else if strings.HasPrefix(relativeNextURL, "http") {
						nextPaginatingURLFromCurrentResponse = relativeNextURL
					} else if relativeNextURL != "" {
						fmt.Printf("\n%s[WARNING] Неожиданный формат URL для следующей страницы: '%s' из блока '%s'. Остановка пагинации для этой даты.%s\n", ColorYellow, relativeNextURL, primaryContentBlock.Alias, ColorReset)
						nextPaginatingURLFromCurrentResponse = ""
						currentPercent := 0
						if quantityLinks > 0 {
							currentPercent = int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
						}
						if currentPercent > 100 {
							currentPercent = 100
						}
						currentCompletedChars := int((float64(currentPercent) / 100.0) * float64(progressBarLength))
						if currentCompletedChars < 0 {
							currentCompletedChars = 0
						}
						if currentCompletedChars > progressBarLength {
							currentCompletedChars = progressBarLength
						}
						currentBar := strings.Repeat("█", currentCompletedChars) + strings.Repeat("-", progressBarLength-currentCompletedChars)
						currentCountStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)
						fmt.Printf("\r[%s] %3d%% %s%s%s", currentBar, currentPercent, ColorGreen, currentCountStr, ColorReset)
					}
				}
			}

			paginatingURL = nextPaginatingURLFromCurrentResponse

			// Обновление прогресс-бара
			percent := 0
			if quantityLinks > 0 { // Предотвращение деления на ноль
				percent = int((float64(len(foundLinks)) / float64(quantityLinks)) * 100)
			}
			if percent > 100 {
				percent = 100
			}

			completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
			if completedChars < 0 {
				completedChars = 0
			}
			if completedChars > progressBarLength {
				completedChars = progressBarLength
			}

			bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
			countStr := fmt.Sprintf("(%d/%d) ", len(foundLinks), quantityLinks)

			// Очищаем предыдущую строку прогресс-бара, чтобы избежать артефактов
			// Длина очистки должна быть достаточной для самой длинной возможной строки прогресс-бара
			clearLength := progressBarLength + len("[]") + len(" 100% ") + len(countStr) + len(ColorGreen) + len(ColorReset) + 5 // +5 на всякий случай
			fmt.Printf("\r%s", strings.Repeat(" ", clearLength))
			fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, ColorGreen, countStr, ColorReset)

			if len(foundLinks) >= quantityLinks {
				break
			}
		}

		if len(foundLinks) >= quantityLinks {
			break
		}
	}

	fmt.Println() // Новая строка после завершения прогресс-бара

	if len(foundLinks) > 0 {
		fmt.Printf("\n%s[INFO] Собрано %d уникальных ссылок на статьи.%s\n", ColorGreen, len(foundLinks), ColorReset)
		//for _, l := range foundLinks {
		//	fmt.Println(l)
		//}
	} else {
		fmt.Printf("\n%s[WARNING] Не найдено ссылок для парсинга (возможно, за указанный период или с текущими фильтрами).%s\n", ColorYellow, ColorReset)
	}

	return parsingPage(foundLinks)
}

func parsingPage(links []string) []Data {
	var articlesData []Data
	var errLinks []string
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
		parsedSuccessfully := false

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
						if classVal, ok := GetAttribute(n, "class"); ok && classVal == "article-main-item__title" {
							title = strings.TrimSpace(ExtractText(n))
						}
					}

					// Поиск контейнера тела статьи
					if body == "" && n.Data == "div" {
						if classVal, ok := GetAttribute(n, "class"); ok && classVal == "article-main-item__body" {
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
				parsedSuccessfully = true
			} else {
				pageStatusMessage = fmt.Sprintf("Нет данных: %s", LimitString(url, 50))
				statusMessageColor = ColorRed
			}
		}

		if !parsedSuccessfully {
			errLinks = append(errLinks, url)
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
		//for idx, product := range articlesData[:10] {
		//	fmt.Printf("\nСтатья #%d\n", idx+1)
		//	fmt.Printf("Заголовок: %s\n", product.Title)
		//	fmt.Printf("Тело:\n%s\n", product.Body)
		//	fmt.Println(strings.Repeat("-", 100))
		//}
		if len(articlesData) < quantityLinks {
			fmt.Printf("\n%s[WARNING] Не собрано %d статей.%s\n", ColorYellow, quantityLinks-len(articlesData), ColorReset)
			for i, el := range errLinks {
				fmt.Printf("%d. %s\n", i+1, el)
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[WARNING] Парсинг статей завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorYellow, totalLinks, ColorReset)
	} else {
		// Этот случай уже обработан в начале функции
	}
	return articlesData
}

// Альтернативная, более короткая версия generateURLForDate с использованием fmt.Sprintf
func generateURLForDateFormatted(date time.Time) string {
	year := fmt.Sprintf("%d", date.Year())
	month := fmt.Sprintf("%02d", date.Month()) // %02d - означает двузначное число, с ведущим нулем если нужно
	day := fmt.Sprintf("%02d", date.Day())     // %02d - означает двузначное число, с ведущим нулем если нужно
	return fmt.Sprintf("%s.%s.%s", day, month, year)
}

// generateURLForPastDate генерирует URL для даты N дней назад
func generateURLForPastDate(daysAgo int) time.Time {
	today := time.Now()
	pastDate := today.AddDate(0, 0, -daysAgo) // Вычитаем дни
	return pastDate
}
