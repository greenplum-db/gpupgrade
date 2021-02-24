# generate tables for scale testing

Note: The script is far from complete, but gives a starting point.

Example:
```
./gen_tables.py -tableType partitionedAO -numOfTables 10 -numOfCols 100 -numOfPartitions 100 -dataType text -outputFile /tmp/output.txt

psql -f /tmp/output.txt <dbname>
```

The above command will generate CREATE and INSERT statement to create 10 root
tables with 100 child partitions each, and 100 columns of datatype text and
will place the output in the file /tmp/output.txt. The generated SQLs can then
be used to load database.

Additional flags:
- tableType: the table type to create, either [partitionedAO, partitionedHeap, partitionedAOCO].
- indexes: whether to create indexes or not
- dataType: the data type to use when creating the columns such as int, text, date, etc.
