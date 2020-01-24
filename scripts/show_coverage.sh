#!/bin/bash

DIR="github.com/greenplum-db/gpupgrade"

# Generate code coverage statistics for one package, write the coverage
# statistics to a file, and print the coverage percentage to the shell
go test -coverpkg $DIR/utils $DIR/utils \
        -coverprofile=/tmp/coverage.out 2> /dev/null \
        | awk '{print $2 " unit test " $4 "\t\t" $5}' \
        | awk -F"/" '{print $4}'

PACKAGES=("agent/services"
"cli/commanders"
"hub/services"
)

for PACKAGE in "${PACKAGES[@]}"; do
  TESTFILE=unit_$(dirname "$PACKAGE")_$(basename "$PACKAGE").out

  # Generate code coverage statistics for the rest of the packages in the same
  # way as above, accounting for the multi-part package specification
  go test -coverpkg "$DIR/$PACKAGE" "$DIR/$PACKAGE" \
          -coverprofile="/tmp/$TESTFILE" \
          | awk '{print $2 " unit test " $4 "\t" $5}' \
          | awk -F"/" '{print $4 "/" $5}'

  # Filter out the first "mode: set" line from each coverage file and
  # concatenate them all
  cat "/tmp/$TESTFILE" | awk '{if($1!="mode:") {print $1 " " $2 " " $3}}' >> /tmp/coverage.out
  rm "/tmp/$TESTFILE"
done

# Print the total coverage percentage and generate a coverage HTML page
go tool cover -func=/tmp/coverage.out | awk '{if($1=="total:") {print $1 "\t\t\t\t\t" $3}}'

echo ""
echo "----------------------"
echo "Show HTML report with:"
echo "$ go tool cover -html /tmp/coverage.out" 
