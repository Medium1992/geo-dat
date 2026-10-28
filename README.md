# geo-dat

`geosite.dat`, `geoip.dat` и `asn.dat` для Xray, собранные из ветки
[`sing`](https://github.com/MetaCubeX/meta-rules-dat/tree/sing) репозитория
MetaCubeX/meta-rules-dat. Сборка идёт раз в сутки и пропускается, если исходник
не изменился.

| Файл | Содержимое | Пример правила |
|---|---|---|
| `geosite.dat` | домены, включая regexp; файлы `name@attr` становятся атрибутами | `geosite:google`, `geosite:google@cn`, `geosite:category-ads-all@ads` |
| `geoip.dat` | страны и служебные списки | `geoip:ru`, `geoip:private`, `!geoip:ru` |
| `asn.dat` | все автономные системы | `ext:asn.dat:AS13335` (регистр не важен) |

## Ссылки

```
https://github.com/Medium1992/geo-dat/releases/latest/download/geosite.dat
https://github.com/Medium1992/geo-dat/releases/latest/download/geoip.dat
https://github.com/Medium1992/geo-dat/releases/latest/download/asn.dat
```

Те же файлы лежат в ветке [`release`](https://github.com/Medium1992/geo-dat/tree/release).
Рядом с каждым файлом есть `*.sha256sum`.

## Xray

Перед первым запуском файлы должны уже лежать в каталоге ассетов Xray:
при загрузке конфига он проверяет, что файл существует и содержит каждый код
из правил.

```json
"geodata": {
  "cron": "0 4 * * *",
  "assets": [
    {"url": "https://github.com/Medium1992/geo-dat/releases/latest/download/geoip.dat",   "file": "geoip.dat"},
    {"url": "https://github.com/Medium1992/geo-dat/releases/latest/download/geosite.dat", "file": "geosite.dat"},
    {"url": "https://github.com/Medium1992/geo-dat/releases/latest/download/asn.dat",     "file": "asn.dat"}
  ],
  "outbound": "direct"
}
```

```json
"rules": [
  {"domain": ["geosite:category-ads-all"], "outboundTag": "block"},
  {"ip": ["ext:asn.dat:AS13335", "ext:asn.dat:AS32934"], "outboundTag": "proxy"},
  {"domain": ["geosite:category-ru"], "ip": ["geoip:ru"], "outboundTag": "direct"}
]
```

## Локальная сборка

```sh
git clone --depth 1 --filter=blob:none --sparse -b sing https://github.com/MetaCubeX/meta-rules-dat.git sing
git -C sing sparse-checkout set --no-cone '/geo/**/*.json' '/asn/*.json'
go run . -src sing -out publish
```

Как переводятся правила sing-box:

| sing-box | Xray |
|---|---|
| `domain` | `full:` |
| `domain_suffix` | `domain:` (`.example.com`: regexp только для поддоменов) |
| `domain_keyword` | `keyword:` |
| `domain_regex` | `regexp:` |
| `ip_cidr` | CIDR |
