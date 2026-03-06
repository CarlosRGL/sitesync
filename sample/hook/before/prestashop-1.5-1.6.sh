#!/bin/bash
set -eu

: "${sqlfile:?sqlfile is required}"

site_slug="${dst_site_slug:-}"
site_slug="/${site_slug#/}"
if [ "$site_slug" = "/" ]; then
	physical_uri="/"
else
	physical_uri="${site_slug%/}/"
fi

MSG=" + Prestashop 1.6 (DB:ps_shop_url.physical_uri)";
echo -n "$MSG"
# MAJ la config des noms de domaine dans la BD
echo "UPDATE ps_shop_url SET physical_uri = '${physical_uri}'; " >> "$sqlfile"
# MAJ pour virer les caches html, css, js
echo "UPDATE ps_configuration SET value = '0' WHERE name = 'PS_CSS_THEME_CACHE'; " >> "$sqlfile"
echo "UPDATE ps_configuration SET value = '0' WHERE name = 'PS_JS_THEME_CACHE'; " >> "$sqlfile"
echo "UPDATE ps_configuration SET value = '0' WHERE name = 'PS_HTML_THEME_COMPRESSION'; " >> "$sqlfile"
echo "UPDATE ps_configuration SET value = '2' WHERE name = 'PS_SMARTY_FORCE_COMPILE'; " >> "$sqlfile"
echo "UPDATE ps_configuration SET value = '0' WHERE name = 'PS_SMARTY_CACHE'; " >> "$sqlfile"
# affichage OK
COL=$((70-${#MSG}))
printf "%${COL}s\n" "OK"
