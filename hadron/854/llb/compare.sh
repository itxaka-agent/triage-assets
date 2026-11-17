#!/bin/sh
# compare.sh DIR A B [regex]
# Prints, for vertices whose name matches regex and that exist in both dumps,
# how many share a digest and which do not.
DIR=$1; A=$2; B=$3; RE=${4:-'^[[]kernel'}
awk -v re="$RE" -v a="$A" -v b="$B" '
FNR==1 { f++ }
{ d=$1; $1=""; n=substr($0,2); if (n !~ re) next; if (f==1) da[n]=d; else db[n]=d }
END {
  same=0; diff=0
  for (n in da) if (n in db) { if (da[n]==db[n]) same++; else { diff++; print "  DIFF " substr(n,1,110) } }
  printf "%s vs %s: %d same, %d differ\n", a, b, same, diff
}' "$DIR/$A.txt" "$DIR/$B.txt"
