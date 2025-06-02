package main

import (
	"fmt"
	"parsing_media/parsers"
	. "parsing_media/utils"
	"sync" // Пакет для синхронизации
)

func main() {

	var num string
	fmt.Print("0 - Run all\n1 - RIA\n2 - Gazeta\n3 - Lenta\n4 - Vesti\n5 - Kommersant\n6 - MK\n7 - Fontanka\n8 - Smotrim\n9 - Banki\n10 - DumaTV\n\n")
	fmt.Scan(&num)

	switch num {
	case "0":
		var wg sync.WaitGroup

		parserJobs := []struct {
			name string
			fn   func()
		}{
			{"RIA", parsers.RiaMain},
			{"Gazeta", parsers.GazetaMain},
			{"Lenta", parsers.LentaMain},
			{"Vesti", parsers.VestiMain},
			{"Kommersant", parsers.KommersMain},
			{"MK", parsers.MKMain},
			{"Fontanka", parsers.FontankaMain},
			{"Smotrim", parsers.SmotrimMain},
			{"Banki", parsers.BankiMain},
			{"DumaTV", parsers.DumaTVMain},
		}

		fmt.Printf("%s[INFO] Запускаем все парсеры%s\n", ColorBlue, ColorReset)

		for _, job := range parserJobs {
			wg.Add(1)

			go func(parserName string, parserFunc func()) {
				defer wg.Done()

				parserFunc()
				fmt.Printf("%s[INFO] Парсер %s завершил работу.%s\n", ColorBlue, parserName, ColorReset)
			}(job.name, job.fn)
		}

		wg.Wait()

		fmt.Printf("%s[INFO] Все парсеры завершили свою работу.%s\n", ColorBlue, ColorReset)

	case "1":
		parsers.RiaMain()

	case "2":
		parsers.GazetaMain()

	case "3":
		parsers.LentaMain()

	case "4":
		parsers.VestiMain()

	case "5":
		parsers.KommersMain()

	case "6":
		parsers.MKMain()

	case "7":
		parsers.FontankaMain()

	case "8":
		parsers.SmotrimMain()

	case "9":
		parsers.BankiMain()

	case "10":
		parsers.DumaTVMain()

	}
}
