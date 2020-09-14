# goKladrService
Сервис для поиска адреса

## Запуск сервиса Elasticsearch синдексом Kladr:
##### Папка "elasticsearch-data" - это индекс Klard для Elasticsearch 

###### new
docker-compose build & docker-compose pull && docker-compose up -d

###### old
```bash
find elasticsearch-data/ -type f -print0 | xargs -0 chmod 666 && \
find elasticsearch-data/ -type d -print0 | xargs -0 chmod 777
docker run -p 9200:9200 -p 9300:9300 -e "discovery.type=single-node" -v ./elasticsearch-data:/usr/share/elasticsearch/data --name elastic docker.elastic.co/elasticsearch/elasticsearch:7.6.0
```

## Запуск API для получения адреса:
```bash
docker build -t go-kladr .
docker run -it -p 8080:8080 --link elastic -e ELASTIC="http://elastic:9200/kladr" go-kladr:latest
```

## Настроки сервиса:
1. Урл для получения статуса - /status
2. Переменная среды ELASTIC - ссылка на сервис Elasticsearch с индексом Kladr (пример: http://localhost:9200/kladr)
3. Переменная среды PORT - порт на котором будет слушать приложение (по умолчанию: 8080)
4. Переменная среды SENTRYURL - урл на Sentry для логирования ошибок

## Входящие параметры:
##### Все параметры необязательные, при использовании двух и более параметров, все они участвуют в поиске.
1. temr - Строка с которой начинается название населенного пункта
2. iterm - Строка которая входит в название населенного пункта
3. region_id - ID региона
4. region_code - Код региона

## Пример работы сервиса
### Запрос:
```bash
curl --request GET --url 'http://localhost:8080/locality?term=%D1%82%D1%8E%D0%BC%D0%B5%D0%BD%D1%8C'
```
### Ответ:
```json
[
  {
    "id": 168105,
    "title": "Тюменская область, г. Тюмень",
    "locality_type": {
      "title": "город"
    },
    "region": {
      "id": 168104,
      "title": "Тюменская область",
      "region_code": 72
    }
  },
  {
    "id": 168547,
    "title": "Тюменская область, Тюменский район, снт. Надежда (30 км трассы Тюмень-Омск)",
    "locality_type": {
      "title": "селонт"
    },
    "region": {
      "id": 168104,
      "title": "Тюменская область",
      "region_code": 72
    }
  },
  {
    "id": 168587,
    "title": "Тюменская область, Тюменский район, снт. Рассвет (15 км а/д Тюмень-Боровский-Бога",
    "locality_type": {
      "title": "селонт"
    },
    "region": {
      "id": 168104,
      "title": "Тюменская область",
      "region_code": 72
    }
  },
  {
    "id": 168169,
    "title": "Тюменская область, снт. Агросад-Тюмень",
    "locality_type": {
      "title": "селонт"
    },
    "region": {
      "id": 168104,
      "title": "Тюменская область",
      "region_code": 72
    }
  },
  {
    "id": 28021,
    "title": "Алтайский край, Троицкий район, с. Тюмень",
    "locality_type": {
      "title": "село"
    },
    "region": {
      "id": 26375,
      "title": "Алтайский край",
      "region_code": 22
    }
  },
  {
    "id": 65670,
    "title": "Иркутская область, Черемховский район, д. Тюмень",
    "locality_type": {
      "title": "деревня"
    },
    "region": {
      "id": 63901,
      "title": "Иркутская область",
      "region_code": 38
    }
  },
  {
    "id": 75659,
    "title": "Кировская область, Оричевский район, д. Тюмень",
    "locality_type": {
      "title": "деревня"
    },
    "region": {
      "id": 72706,
      "title": "Кировская область",
      "region_code": 43
    }
  },
  {
    "id": 124715,
    "title": "Пермский край, Юсьвинский район, д. Тюмень",
    "locality_type": {
      "title": "деревня"
    },
    "region": {
      "id": 120145,
      "title": "Пермский край",
      "region_code": 59
    }
  }
]
```

## Реализация функционала FOR_SELECT (/api/kladr/for_select/)

## Входящие параметры:
##### Все параметры необязательные, при использовании двух и более параметров, все они участвуют в поиске.
1. search - Строка с которой начинается название населенного пункта, может состоять из двух слов.
2. regions_only - если =1, то ищет только среди регионов
3. cities_and_regions - если =1, то ищет только среди городов и регионов

## Пример работы сервиса
### Запрос:
```bash
curl --request GET --url 'http://localhost:8080/api/kladr/for_select/?search=%D0%BD%D0%B8%D0%B6%D0%BD%D0%B8%D0%B9%20%D0%BD%D0%BE%D0%B2'
```
### Ответ:
```json
{
"count": 4,
"next": null,
"previous": null,
"results": [
{
"id": 100558,
"text": "Нижний Новгород город"
},
{
"id": 97928,
"text": "74 км ш.Москва-Нижний Новгород населенный пункт"
},
{
"id": 97929,
"text": "73 км ш.Москва-Нижний Новгород населенный пункт"
},
{
"id": 116006,
"text": "Нижний Жерновец село"
}
]
}

```