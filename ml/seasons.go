package ml

import (
	"fmt"
	"time"
)

type SeasonContext struct {
	SeasonName string
	MonthName  string
	Holidays   string
	Hints      string
}

func (s SeasonContext) String() string {
	return fmt.Sprintf("Сезон: %s (%s). Праздники и контекст: %s. Подсказки: %s", s.SeasonName, s.MonthName, s.Holidays, s.Hints)
}

// GetSeasonContext returns seasonal and holiday context for the specified date
func GetSeasonContext(t time.Time) SeasonContext {
	month := t.Month()
	day := t.Day()

	switch month {
	case time.December:
		if day >= 20 {
			return SeasonContext{
				SeasonName: "Зима",
				MonthName:  "Декабрь",
				Holidays:   "Преддверие Нового года и католическое Рождество",
				Hints:      "Новогоднее волшебство, рождественские семейные комедии, сказочное фэнтези, уютные снежные сказки, праздничные мелодрамы",
			}
		}
		return SeasonContext{
			SeasonName: "Зима",
			MonthName:  "Декабрь",
			Holidays:   "Зимний сезон, подготовка к Новому году",
			Hints:      "Снежные приключения, выживание в ледяных снегах, зимние комедии, ожидание чуда, согревающие драмы",
		}
	case time.January:
		return SeasonContext{
			SeasonName: "Зима",
			MonthName:  "Январь",
			Holidays:   "Новый год, православное Рождество, зимние каникулы",
			Hints:      "Зимняя классика, новогодние комедии, снежные триллеры, фильмы про выживание в морозы, теплое кино для всей семьи",
		}
	case time.February:
		if day >= 10 && day <= 16 {
			return SeasonContext{
				SeasonName: "Зима",
				MonthName:  "Февраль",
				Holidays:   "День святого Валентина (День влюбленных)",
				Hints:      "Романтические мелодрамы, страстные истории любви, романтические комедии для пар, культовые фильмы про отношения",
			}
		} else if day >= 20 {
			return SeasonContext{
				SeasonName: "Зима",
				MonthName:  "Февраль",
				Holidays:   "День защитника Отечества",
				Hints:      "Мужественные герои, военные боевики, спецназ и спецоперации, исторические сражения, несгибаемый характер",
			}
		}
		return SeasonContext{
			SeasonName: "Зима",
			MonthName:  "Февраль",
			Holidays:   "Конец зимы",
			Hints:      "Снежный нуар, северные детективы, ледяные триллеры, фильмы про выносливость",
		}
	case time.March:
		if day <= 10 {
			return SeasonContext{
				SeasonName: "Весна",
				MonthName:  "Март",
				Holidays:   "Международный женский день (8 Марта)",
				Hints:      "Великие женщины в кино, сильные женские персонажи, женские детективы, весенние романтические истории",
			}
		}
		return SeasonContext{
			SeasonName: "Весна",
			MonthName:  "Март",
			Holidays:   "Начало весны, таяние снегов",
			Hints:      "Фильмы про перемены и новые начинания, весенний Париж, вдохновляющие драмы, легкие европейские комедии",
		}
	case time.April:
		if day >= 10 && day <= 15 {
			return SeasonContext{
				SeasonName: "Весна",
				MonthName:  "Апрель",
				Holidays:   "День космонавтики (12 апреля)",
				Hints:      "Покорение космоса, научная фантастика, Гагарин и первые полеты, межзвездные путешествия, тайны вселенной",
			}
		}
		return SeasonContext{
			SeasonName: "Весна",
			MonthName:  "Апрель",
			Holidays:   "Расцвет весны",
			Hints:      "Весеннее пробуждение, дорожные приключения, интеллектуальное кино, фильмы-загадки",
		}
	case time.May:
		if day <= 12 {
			return SeasonContext{
				SeasonName: "Весна",
				MonthName:  "Май",
				Holidays:   "День Победы (9 Мая) и майские праздники",
				Hints:      "Великие фильмы о войне, бессмертный подвиг, советская военная классика, фильмы о победе и стойкости духа",
			}
		}
		return SeasonContext{
			SeasonName: "Весна",
			MonthName:  "Май",
			Holidays:   "Конец весны, преддверие лета",
			Hints:      "Дачные истории, легкие комедии, приключения на природе, первые летние блокбастеры",
		}
	case time.June:
		return SeasonContext{
			SeasonName: "Лето",
			MonthName:  "Июнь",
			Holidays:   "Начало лета, День защиты детей",
			Hints:      "Детские приключения, летние каникулы, семейная анимация, подростковые комедии, солнечные истории",
		}
	case time.July:
		return SeasonContext{
			SeasonName: "Лето",
			MonthName:  "Июль",
			Holidays:   "Разгар лета, сезон отпусков",
			Hints:      "Жаркие пляжные триллеры, фильмы про акул и океан, роуд-муви, приключения в джунглях, громкие блокбастеры",
		}
	case time.August:
		return SeasonContext{
			SeasonName: "Лето",
			MonthName:  "Август",
			Holidays:   "Конец лета, День российского кино (27 августа)",
			Hints:      "Золотой фонд советского и российского кино, ностальгические летние драмы, романтические закаты, культовые комедии Гайдая и Данелии",
		}
	case time.September:
		return SeasonContext{
			SeasonName: "Осень",
			MonthName:  "Сентябрь",
			Holidays:   "Начало осени, День знаний",
			Hints:      "Школьные и студенческие истории, академия и ученые, уютные осенние детективы, фильмы с меланхоличным джазовым настроением",
		}
	case time.October:
		if day >= 20 {
			return SeasonContext{
				SeasonName: "Осень",
				MonthName:  "Октябрь",
				Holidays:   "Хэллоуин, канун Дня всех святых",
				Hints:      "Культовые хорроры, готические замки, вампиры и ведьмы, жуткие городские легенды, атмосферные мистические триллеры",
			}
		}
		return SeasonContext{
			SeasonName: "Осень",
			MonthName:  "Октябрь",
			Holidays:   "Золотая осень",
			Hints:      "Психологические триллеры, дождливые детективы, уютные драмы с пледом и чаем, философские притчи",
		}
	case time.November:
		return SeasonContext{
			SeasonName: "Осень",
			MonthName:  "Ноябрь",
			Holidays:   "Поздняя осень, темные вечера",
			Hints:      "Мрачный неонуар, расследования сыщиков под дождем, интеллектуальные триллеры-головоломки, фильмы про ограбления и мафию",
		}
	}
	return SeasonContext{
		SeasonName: "Сезон кино",
		MonthName:  "Текущий месяц",
		Holidays:   "Киновыходные",
		Hints:      "Увлекательные сюжеты, мировые хиты, авторские находки, приключения для всей семьи",
	}
}
