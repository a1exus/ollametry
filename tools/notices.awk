# Turns `go-licenses report` CSV (package,license URL,license) into Markdown
# table rows, one per package with its licenses merged and sorted. Versions
# are left out on purpose so dependency bumps do not change the notices.
$1 != "github.com/a1exus/ollametry" && NF >= 3 {
	if (!($1 in licenses)) {
		order[++n] = $1
		licenses[$1] = $3
	} else if (index(", " licenses[$1] ", ", ", " $3 ", ") == 0) {
		licenses[$1] = licenses[$1] ", " $3
	}
}
END {
	print "| Package | License |"
	print "| --- | --- |"
	for (i = 1; i <= n; i++) {
		pkg = order[i]
		printf "| [`%s`](https://pkg.go.dev/%s?tab=licenses) | %s |\n", pkg, pkg, licenses[pkg]
	}
}
