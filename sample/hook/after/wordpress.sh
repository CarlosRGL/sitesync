#!/bin/bash
set -eu

: "${src_site_host:?src_site_host is required}"
: "${dst_site_host:?dst_site_host is required}"
: "${dst_files_root:?dst_files_root is required}"
: "${dst_path_to_resilient_replace:?dst_path_to_resilient_replace is required}"

site_slug="${dst_site_slug:-}"
site_slug="/${site_slug#/}"
if [ "$site_slug" = "/" ]; then
	rewrite_base="/"
	error_document="/index.php"
else
	rewrite_base="${site_slug%/}/"
	error_document="${rewrite_base}index.php"
fi

run_replace() {
	$dst_path_to_resilient_replace "$1" "$2" "$3"
}

MSG=" + Wordpress (.htaccess)";
echo -n "$MSG"
# MAJ la config des noms de domaine dans le .htaccess et vide les caches
run_replace "RewriteCond %{HTTP_HOST} \\^${src_site_host}\\$" "RewriteCond %{HTTP_HOST} ^${dst_site_host}$" "${dst_files_root}/.htaccess"
run_replace "RewriteCond %{HTTP_HOST} \\^www.${src_site_host}\\$" "RewriteCond %{HTTP_HOST} ^${dst_site_host}$" "${dst_files_root}/.htaccess"
run_replace "\[E=REWRITEBASE:\/\]" "[E=REWRITEBASE:${rewrite_base}]" "${dst_files_root}/.htaccess"
run_replace "ErrorDocument 404 \/index\.php" "ErrorDocument 404 ${error_document}" "${dst_files_root}/.htaccess"
# problemes de droits ?
# chmod -R 777 ${dst_files_root}/cache/smarty/compile
# chmod -R 777 ${dst_files_root}/cache/cachefs
# chmod -R o+rx ${dst_files_root}
# affichage OK
COL=$((70-${#MSG}))
printf "%${COL}s\n" "OK"
