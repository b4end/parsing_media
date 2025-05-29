package dumatv

import (
	"encoding/json"
	"fmt"
	. "parsing_media/utils"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// DumaTVArticleItem определяет структуру для одного элемента (статьи/новости)
// в списке из JSON API dumatv.ru
type DumaTVArticleItem struct {
	ID              string `json:"id"`              // Идентификатор статьи, например, "83549"
	Title           string `json:"title"`           // Заголовок статьи
	Time            string `json:"time"`            // Время публикации, например, "17:42"
	Link            string `json:"link"`            // Полная ссылка на статью
	IsDumaTvArticle bool   `json:"isDumaTvArticle"` // Является ли статья непосредственно с dumatv.ru
}

// DumaTVAPIResponse определяет общую структуру ответа JSON API dumatv.ru
type DumaTVAPIResponse struct {
	CurrentDate          string              `json:"current_date"`           // Текущая дата в формате "DD.MM.YYYY"
	NextDate             string              `json:"next_date"`              // Следующая (или предыдущая, в зависимости от контекста API) дата в формате "DD.MM.YYYY"
	CurrentDateFormatted string              `json:"current_date_formatted"` // Текущая дата в отформатированном виде, например, "29 мая 2025"
	NextDateFormatted    string              `json:"next_date_formatted"`    // Следующая (или предыдущая) дата в отформатированном виде
	List                 []DumaTVArticleItem `json:"list"`                   // Список статей/новостей
}

const (
	quantityLinks    = 100
	dumatvAPIBaseURL = "https://dumatv.ru/api/news-home?date=%s"
	initialDaysAgo   = 0
)

func DumaTVMain() {
	totalStartTime := time.Now()

	fmt.Printf("%s[INFO] Запуск программы...%s\n", ColorYellow, ColorReset)
	_ = parsingLinks() // Сохраняем результат для возможного использования

	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("\n%s[INFO] Общее время выполнения программы: %s%s\n", ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

func parsingLinks() []Data {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	fmt.Printf("\n%s[INFO] Начало сбора ссылок на статьи с DumaTV API...%s\n", ColorYellow, ColorReset)
	progressBarLength := 40
	updateProgressBar := func(currentFound, totalNeeded int) {
		percent := 0
		if totalNeeded > 0 {
			percent = int((float64(currentFound) / float64(totalNeeded)) * 100)
		}
		if percent > 100 {
			percent = 100
		}
		completedChars := int((float64(percent) / 100.0) * float64(progressBarLength))
		if completedChars < 0 {
			completedChars = 0
		} else if completedChars > progressBarLength {
			completedChars = progressBarLength
		}
		bar := strings.Repeat("█", completedChars) + strings.Repeat("-", progressBarLength-completedChars)
		countStr := fmt.Sprintf("(%d/%d) ", currentFound, totalNeeded)
		clearLength := progressBarLength + len("[]") + len(" 100% ") + len(countStr) + len(ColorGreen) + len(ColorReset) + 5
		fmt.Printf("\r%s", strings.Repeat(" ", clearLength))
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, ColorGreen, countStr, ColorReset)
	}

	updateProgressBar(len(foundLinks), quantityLinks)
	initialDateStr := generateURLForDateFormatted(generateURLForPastDate(initialDaysAgo))
	paginatingURL := fmt.Sprintf(dumatvAPIBaseURL, initialDateStr)

	for paginatingURL != "" && len(foundLinks) < quantityLinks {
		rawJSONData, err := GetJSON(paginatingURL)
		if err != nil {
			fmt.Printf("\n%s[CRITICAL] Не удалось получить JSON с %s. Ошибка: %s. Остановка сбора ссылок.%s\n", ColorRed, paginatingURL, err, ColorReset)
			paginatingURL = ""
			updateProgressBar(len(foundLinks), quantityLinks)
			continue
		}
		var apiResponse DumaTVAPIResponse
		jsonBytes, err := json.Marshal(rawJSONData)
		if err != nil {
			fmt.Printf("\n%s[ERROR] Не удалось пере-маршализовать JSON для URL %s. Ошибка: %s. Остановка сбора ссылок.%s\n", ColorRed, paginatingURL, err, ColorReset)
			paginatingURL = ""
			updateProgressBar(len(foundLinks), quantityLinks)
			break
		}
		err = json.Unmarshal(jsonBytes, &apiResponse)
		if err != nil {
			fmt.Printf("\n%s[CRITICAL] Не удалось преобразовать JSON в структуру DumaTVAPIResponse для URL %s. Ошибка: %s. Остановка сбора ссылок.%s\n", ColorRed, paginatingURL, err, ColorReset)
			paginatingURL = ""
			updateProgressBar(len(foundLinks), quantityLinks)
			break
		}

		for _, item := range apiResponse.List {
			if len(foundLinks) >= quantityLinks {
				break
			}
			if item.Link == "" {
				continue
			}
			fullHref := item.Link
			if fullHref != "" && !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}

		if apiResponse.NextDate != "" && len(foundLinks) < quantityLinks {
			paginatingURL = fmt.Sprintf(dumatvAPIBaseURL, apiResponse.NextDate)
		} else {
			paginatingURL = ""
		}
		updateProgressBar(len(foundLinks), quantityLinks)
		if len(foundLinks) >= quantityLinks {
			paginatingURL = ""
		}
	}
	fmt.Println()
	if len(foundLinks) > 0 {
		fmt.Printf("\n%s[INFO] Собрано %d уникальных ссылок на статьи.%s\n", ColorGreen, len(foundLinks), ColorReset)
	} else {
		fmt.Printf("\n%s[WARNING] Не найдено ссылок для парсинга.%s\n", ColorYellow, ColorReset)
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
		var title, body, pageDate, pageTime string // Добавлены pageDate, pageTime
		var pageStatusMessage string
		var statusMessageColor = ColorReset
		parsedSuccessfully := false

		doc, err := GetHTML(url)
		if err != nil {
			pageStatusMessage = fmt.Sprintf("Ошибка GET: %s", LimitString(err.Error(), 50))
			statusMessageColor = ColorRed
		} else {
			var extractData func(*html.Node)
			extractData = func(n *html.Node) {
				// Если все данные уже найдены, дальше не ищем
				if title != "" && body != "" && pageDate != "" && pageTime != "" {
					return
				}

				if n.Type == html.ElementNode {
					// Поиск заголовка
					if title == "" && n.Data == "h1" {
						if classVal, ok := GetAttribute(n, "class"); ok && classVal == "news-post-content__title" {
							title = strings.TrimSpace(ExtractText(n))
						}
					}

					// Поиск даты и времени
					if (pageDate == "" || pageTime == "") && n.Data == "div" {
						if classVal, ok := GetAttribute(n, "class"); ok && classVal == "news-post-top__date" {
							rawDateTimeStr := strings.TrimSpace(ExtractText(n))
							parts := strings.Split(rawDateTimeStr, " / ")
							if len(parts) == 2 {
								pageDate = strings.TrimSpace(parts[0])
								pageTime = strings.TrimSpace(parts[1])
							} else {
								// Можно добавить лог, если формат даты/времени неожиданный
								fmt.Printf("\n%s[WARN] Неожиданный формат даты/времени: '%s' на %s%s\n", ColorYellow, rawDateTimeStr, url, ColorReset)
							}
						}
					}

					// Поиск контейнера тела статьи
					if body == "" && n.Data == "div" {
						if classVal, ok := GetAttribute(n, "class"); ok && classVal == "news-post-content__text" {
							var bodyBuilder strings.Builder
							for child := n.FirstChild; child != nil; child = child.NextSibling {
								if child.Type == html.ElementNode {
									if child.Data == "div" {
										if childClass, classOk := GetAttribute(child, "class"); classOk && strings.Contains(childClass, "sharing-panel") {
											continue
										}
									}
									if child.Data == "p" || child.Data == "blockquote" {
										paragraphText := strings.TrimSpace(ExtractText(child))
										if paragraphText != "" {
											if bodyBuilder.Len() > 0 {
												bodyBuilder.WriteString("\n\n")
											}
											bodyBuilder.WriteString(paragraphText)
										}
									}
								}
							}
							body = bodyBuilder.String()
						}
					}
				}

				// Продолжаем рекурсию по дочерним узлам, если что-то еще не найдено
				if title == "" || body == "" || pageDate == "" || pageTime == "" {
					for c := n.FirstChild; c != nil; c = c.NextSibling {
						extractData(c)
						if title != "" && body != "" && pageDate != "" && pageTime != "" {
							break
						}
					}
				}
			}
			extractData(doc)

			if title != "" && body != "" && pageDate != "" && pageTime != "" {
				articlesData = append(articlesData, Data{
					Title: title,
					Body:  body,
					Href:  url,      // Добавляем ссылку
					Date:  pageDate, // Добавляем дату
					Time:  pageTime, // Добавляем время
				})
				pageStatusMessage = fmt.Sprintf("Успех: %s", LimitString(title, 50))
				statusMessageColor = ColorGreen
				parsedSuccessfully = true
			} else {
				// Уточненное сообщение об ошибке
				errMsg := fmt.Sprintf("T:%t|B:%t|D:%t|Ti:%t", title != "", body != "", pageDate != "", pageTime != "")
				pageStatusMessage = fmt.Sprintf("Нет данных [%s]: %s", errMsg, LimitString(url, 35))
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
		availableWidthForMessage := statusTextWidth - len(countStr)
		if availableWidthForMessage < 10 {
			availableWidthForMessage = 10
		}
		displayMessage := LimitString(pageStatusMessage, availableWidthForMessage)
		fullStatusText := countStr + displayMessage
		if len(fullStatusText) < statusTextWidth {
			fullStatusText += strings.Repeat(" ", statusTextWidth-len(fullStatusText))
		} else if len(fullStatusText) > statusTextWidth {
			fullStatusText = fullStatusText[:statusTextWidth]
		}
		fmt.Printf("\r[%s] %3d%% %s%s%s", bar, percent, statusMessageColor, fullStatusText, ColorReset)
	}

	fmt.Println()

	if len(articlesData) > 0 {
		fmt.Printf("\n\n%s[INFO] Парсинг статей завершен. Собрано %d статей.%s\n", ColorGreen, len(articlesData), ColorReset)
		// Раскомментируйте для вывода всех собранных статей
		/*
			for idx, product := range articlesData {
				fmt.Printf("\nСтатья #%d\n", idx+1)
				fmt.Printf("Ссылка: %s\n", product.Href)
				fmt.Printf("Заголовок: %s\n", product.Title)
				fmt.Printf("Дата: %s\n", product.Date)
				fmt.Printf("Время: %s\n", product.Time)
				fmt.Printf("Тело:\n%s\n", product.Body)
				fmt.Println(strings.Repeat("-", 100))
			}
		*/
		if len(articlesData) < totalLinks {
			fmt.Printf("\n%s[WARNING] Не удалось обработать %d из %d ссылок.%s\n", ColorYellow, totalLinks-len(articlesData), totalLinks, ColorReset)
			if len(errLinks) > 0 {
				fmt.Printf("%s[INFO] Список ссылок с ошибками или без данных:%s\n", ColorYellow, ColorReset)
				for i, el := range errLinks {
					fmt.Printf("%d. %s\n", i+1, el)
				}
			}
		}
	} else if totalLinks > 0 {
		fmt.Printf("\n%s[WARNING] Парсинг статей завершен, но не удалось собрать данные ни с одной из %d страниц.%s\n", ColorYellow, totalLinks, ColorReset)
	}
	return articlesData
}

// generateURLForDateFormatted (без изменений)
func generateURLForDateFormatted(date time.Time) string {
	year := fmt.Sprintf("%d", date.Year())
	month := fmt.Sprintf("%02d", date.Month())
	day := fmt.Sprintf("%02d", date.Day())
	return fmt.Sprintf("%s.%s.%s", day, month, year)
}

// generateURLForPastDate (без изменений)
func generateURLForPastDate(daysAgo int) time.Time {
	today := time.Now()
	pastDate := today.AddDate(0, 0, -daysAgo)
	return pastDate
}
