// parsers/banki.go
package parsers

import (
	"encoding/json"
	"fmt"
	"net/http"
	. "parsing_media/utils" // Используем dot import для прямого доступа к функциям из utils
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Структуры для парсинга JSON с главной страницы новостей
type NewsItemJSONBanki struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type ListViewItemsJSONBanki map[string][]NewsItemJSONBanki

type ModuleOptionsJSONBanki struct {
	ListViewItems ListViewItemsJSONBanki `json:"listViewItems"`
	PageRoute     string                 `json:"pageRoute"`
}

// Константы для парсера Banki.ru
const (
	bankiURL        = "https://www.banki.ru"
	bankiURLNews    = "https://www.banki.ru/news/lenta/"
	numWorkersBanki = 10 // Количество одновременных воркеров для парсинга статей
)

// BankiMain - основная функция, запускающая парсер
func BankiMain() {
	totalStartTime := time.Now()
	// 1. Получаем список ссылок на статьи
	foundLinks := getLinksBanki()
	// 2. Парсим каждую статью из списка
	products := getPageBanki(foundLinks)
	// 3. Сохраняем результат в базу данных, если что-то найдено
	if len(products) > 0 {
		SaveData(products)
	}
	totalElapsedTime := time.Since(totalStartTime)
	fmt.Printf("%s[BANKI]%s[INFO] Парсер Banki.ru заверщил работу: (%s)%s\n", ColorBlue, ColorYellow, FormatDuration(totalElapsedTime), ColorReset)
}

// getLinksBanki получает со страницы banki.ru/news/lenta/ список новостных ссылок
func getLinksBanki() []string {
	var foundLinks []string
	seenLinks := make(map[string]bool)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	doc, err := GetHTMLForClient(client, bankiURLNews)
	if err != nil {
		fmt.Printf("%s[BANKI]%s[ERROR] Ошибка при получении HTML со страницы %s: %v%s\n", ColorBlue, ColorRed, bankiURLNews, err, ColorReset)
		return foundLinks
	}

	var jsonData string
	doc.Find("div[data-module*='NewsBundle/app/desktop/lenta-list']").EachWithBreak(func(i int, s *goquery.Selection) bool {
		optionsStr, exists := s.Attr("data-module-options")
		if exists {
			jsonData = optionsStr
			return false
		}
		return true
	})

	if jsonData == "" {
		fmt.Printf("%s[BANKI]%s[WARNING] Не найден JSON с данными о новостях на странице %s.%s\n", ColorBlue, ColorYellow, bankiURLNews, ColorReset)
		return foundLinks
	}

	var moduleOpts ModuleOptionsJSONBanki
	err = json.Unmarshal([]byte(jsonData), &moduleOpts)
	if err != nil {
		fmt.Printf("%s[BANKI]%s[ERROR] Не удалось распарсить JSON со страницы %s: %v%s\n", ColorBlue, ColorRed, bankiURLNews, err, ColorReset)
		return foundLinks
	}

	if len(moduleOpts.ListViewItems) == 0 {
		fmt.Printf("%s[BANKI]%s[INFO] В JSON поле listViewItems пусто на %s.%s\n", ColorBlue, ColorYellow, bankiURLNews, ColorReset)
		return foundLinks
	}

	pageRoute := "/news/daytheme/"

	for _, newsItemsOnDate := range moduleOpts.ListViewItems {
		for _, item := range newsItemsOnDate {
			fullHref := fmt.Sprintf("%s%s?id=%d", bankiURL, pageRoute, item.ID)
			if !seenLinks[fullHref] {
				seenLinks[fullHref] = true
				foundLinks = append(foundLinks, fullHref)
			}
		}
	}

	if len(foundLinks) <= 0 {
		fmt.Printf("%s[BANKI]%s[WARNING] Не найдено ссылок для парсинга из JSON на %s.%s\n", ColorBlue, ColorYellow, bankiURLNews, ColorReset)
	}

	return foundLinks
}

// Структура для передачи результатов между горутинами
type pageParseResultBanki struct {
	Data    Data
	Error   error
	PageURL string
	IsEmpty bool
	Reasons []string
}

// getPageBanki парсит каждую страницу из переданного списка ссылок
func getPageBanki(links []string) []Data {
	var products []Data
	var errItems []string
	totalLinks := len(links)
	locationPlus3 := time.FixedZone("UTC+3", 3*3600) // Время на сайте московское

	if totalLinks == 0 {
		return products
	}

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	resultsChan := make(chan pageParseResultBanki, totalLinks)
	linkChan := make(chan string, totalLinks)

	for _, link := range links {
		linkChan <- link
	}
	close(linkChan)

	var wg sync.WaitGroup

	actualNumWorkers := numWorkersBanki
	if totalLinks < numWorkersBanki {
		actualNumWorkers = totalLinks
	}

	// Запускаем воркеров для параллельного парсинга
	for i := 0; i < actualNumWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pageURL := range linkChan {
				var title, body string
				var parsDate time.Time
				var dateStr string

				doc, err := GetHTMLForClient(httpClient, pageURL)
				if err != nil {
					resultsChan <- pageParseResultBanki{PageURL: pageURL, Error: fmt.Errorf("ошибка GET: %w", err)}
					continue
				}

				// --- НАЧАЛО ИЗМЕНЕНИЙ: УНИВЕРСАЛЬНАЯ ЛОГИКА ПАРСИНГА ДЛЯ 3-Х ШАБЛОНОВ ---

				// 1. Пытаемся получить заголовок, проверяя 3 разных селектора
				title = strings.TrimSpace(doc.Find("h1[data-qa-id=\"title\"]").First().Text()) // Шаблон №3 (новый)
				if title == "" {
					title = strings.TrimSpace(doc.Find("h1[class*=\"header_\"]").First().Text()) // Шаблон №2
				}
				if title == "" {
					title = strings.TrimSpace(doc.Find("h1.text-header-0").First().Text()) // Шаблон №1 (старый)
				}

				// 2. Пытаемся получить тело статьи, проверяя 3 разных селектора
				var bodyBuilder strings.Builder
				bodySelection := doc.Find("div.article-body") // Шаблон №3 (новый)
				if bodySelection.Length() == 0 {
					bodySelection = doc.Find("div[data-qa-id=\"article-body\"]") // Шаблон №2
				}
				if bodySelection.Length() == 0 {
					bodySelection = doc.Find("div.l6d291019") // Шаблон №1 (старый)
				}

				bodySelection.Find("p, li").Each(func(_ int, s *goquery.Selection) {
					partText := strings.TrimSpace(s.Text())
					if partText != "" && !strings.Contains(partText, "Самый большой финансовый маркетплейс в России") {
						if bodyBuilder.Len() > 0 {
							bodyBuilder.WriteString("\n\n")
						}
						bodyBuilder.WriteString(partText)
					}
				})
				body = bodyBuilder.String()

				// 3. Пытаемся получить дату, проверяя 3 разных селектора
				dateStr = strings.TrimSpace(doc.Find("div[data-qa-id=\"meta-item\"]").First().Text()) // Шаблон №3 (новый)
				if dateStr == "" {
					dateStr = strings.TrimSpace(doc.Find("div[class*=\"meta_\"] span").First().Text()) // Шаблон №2
				}
				if dateStr == "" {
					dateStr = strings.TrimSpace(doc.Find("span[class*='l51e0a7a5']").First().Text()) // Шаблон №1 (старый)
				}

				dateToParse := strings.TrimPrefix(dateStr, "Дата публикации: ")
				if dateToParse != "" {
					parsedTime, parseErr := time.ParseInLocation("02.01.2006 15:04", dateToParse, locationPlus3)
					if parseErr == nil {
						parsDate = parsedTime
					}
				}
				// --- КОНЕЦ ИЗМЕНЕНИЙ ---

				if title != "" && body != "" && !parsDate.IsZero() {
					dataItem := Data{
						Site: bankiURL, Href: pageURL, Title: title, Body: body, Date: parsDate,
					}
					hash, err := dataItem.Hashing()
					if err != nil {
						resultsChan <- pageParseResultBanki{PageURL: pageURL, Error: fmt.Errorf("ошибка генерации хеша: %w", err)}
						continue
					}
					dataItem.Hash = hash
					resultsChan <- pageParseResultBanki{Data: dataItem}
				} else {
					var reasons []string
					if title == "" {
						reasons = append(reasons, "T:false")
					}
					if body == "" {
						reasons = append(reasons, "B:false")
					}
					if parsDate.IsZero() {
						reasons = append(reasons, "D:false (исходная строка: '"+dateToParse+"')")
					}
					resultsChan <- pageParseResultBanki{PageURL: pageURL, IsEmpty: true, Reasons: reasons}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		if result.Error != nil {
			errItems = append(errItems, fmt.Sprintf("%s (%s)", result.PageURL, result.Error.Error()))
		} else if result.IsEmpty {
			errItems = append(errItems, fmt.Sprintf("%s (нет данных: %s)", result.PageURL, strings.Join(result.Reasons, ", ")))
		} else {
			products = append(products, result.Data)
		}
	}

	if len(products) > 0 {
		fmt.Printf("%s[BANKI]%s[INFO] Успешно собрано %d статей.%s\n", ColorBlue, ColorGreen, len(products), ColorReset)
	}

	if len(errItems) > 0 {
		if len(products) == 0 {
			fmt.Printf("%s[BANKI]%s[ERROR] Не удалось собрать данные ни с одной из %d страниц.%s\n", ColorBlue, ColorRed, totalLinks, ColorReset)
		} else {
			fmt.Printf("%s[BANKI]%s[WARNING] Не удалось обработать %d из %d страниц:%s\n", ColorBlue, ColorYellow, len(errItems), totalLinks, ColorReset)
		}
		fmt.Printf("%s[BANKI]%s[INFO] Список страниц с ошибками:%s\n", ColorBlue, ColorYellow, ColorReset)
		for idx, itemMessage := range errItems {
			if idx >= 10 {
				fmt.Printf("%s  ... и еще %d ошибок ...%s\n", ColorYellow, len(errItems)-10, ColorReset)
				break
			}
			fmt.Printf("%s  %d. %s%s\n", ColorYellow, idx+1, itemMessage, ColorReset)
		}
	}

	return products
}
