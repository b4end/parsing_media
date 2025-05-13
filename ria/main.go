package main

import (
	"fmt"
	"net/http"

	"golang.org/x/net/html"
)

// Product определяет структуру для хранения данных о продукте.
type Data struct {
	Title string
	Body  string
}

func main() {
	parsing_links()
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

func parsing_links() {
	// URL ленты новостей
	URL := "https://ria.ru/lenta/"

	// Получаем HTML-документ ленты новостей
	doc, err := get_html(URL)
	if err != nil {
		fmt.Printf("Ошибка при получении HTML со страницы %s: %v\n", URL, err)
		return
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
				foundLinks = append(foundLinks, hrefValue) // Добавляем "как есть"
			}
		}

		// Рекурсивно обходим всех потомков текущего узла
		for c := h.FirstChild; c != nil; c = c.NextSibling {
			extractLinks(c)
		}
	}

	// Запускаем обход HTML-дерева
	extractLinks(doc)

	// Выводим результат
	if len(foundLinks) > 0 {
		fmt.Printf("/nНайдены ссылки с классом '%s':\n", "list-item__title color-font-hover-only")
		for _, link := range foundLinks {
			fmt.Println(link)
		}
	} else {
		fmt.Printf("/nНе найдено ссылок с классом '%s' на странице %s.\n", "list-item__title color-font-hover-only", URL)
		fmt.Println("Возможные причины:")
		fmt.Println("1. HTML-структура сайта изменилась, и элементы <a> с таким точным классом отсутствуют.")
		fmt.Println("2. Элементы с таким классом загружаются динамически с помощью JavaScript (маловероятно для основного списка новостей РИА).")
		fmt.Println("3. Ошибка в URL или временные проблемы с доступом к сайту (проверьте вывод ошибки от get_html).")
		fmt.Println("Рекомендуется проверить актуальную HTML-структуру страницы в браузере (Инструменты разработчика -> Исследовать элемент).")
	}

}
