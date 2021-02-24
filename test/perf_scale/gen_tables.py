#!/usr/bin/env python

import argparse

def genColumns(count, dataType):
    colString = ""
    for i in range(count):
        colType = dataType
        if (i < 2):
            colType = "int"
        col = "col{0} {1}".format(i, colType)
        if (i == 0):
            colString = col
        if (i < count and i != 0):
            colString = colString + "," + col
    return colString


def createTable(prefix, count, indexes):
    tableStrings = []
    insertIntos = []
    indexesStmt = []
    for i in range(count):
        tableName = "{0}_table{1}".format(prefix, i)
        tableString = "CREATE TABLE {0}".format(tableName)
        tableStrings.append(tableString)
        insertInto = "INSERT INTO {0} SELECT ".format(tableName)
        insertIntos.append(insertInto)
        if indexes:
            indexstmt = "CREATE INDEX {0}_idx ON {0}(col2);".format(tableName)
            indexesStmt.append(indexstmt)


    return tableStrings, insertIntos, indexesStmt


def genCreateStmt(tablePrefix, tableCount, numOfPartitions, colCount, dataType, tableType, indexes, outputFile):
    ddls = []
    tableStrings, insertIntos, indexStmt = createTable(tablePrefix, tableCount, indexes)
    colString = genColumns(colCount, dataType)
    t = ""
    if tableType == "heap":
        t = "DISTRIBUTED RANDOMLY"
    if tableType == "partitionedHeap":
        t = "{0} PARTITION BY RANGE(col1) (START(0) END({1}) EVERY(1))".format(t, numOfPartitions)
    if tableType == "partitionedAO":
        t = "WITH (APPENDONLY=true) {0} PARTITION BY RANGE(col1) (START(0) END({1}) EVERY(1))".format(t, numOfPartitions)
    elif tableType == "partitionedAOCO":
        t = "WITH (orientation='column', APPENDONLY=true) {0} PARTITION BY RANGE(col1) (START(0) END({1}) EVERY(1))".format(t, numOfPartitions)
      
    for tableString in tableStrings:
        ddl = "{0} ({1}) {2};".format(tableString, colString, t)
        ddls.append(ddl)
    valString = ""
    for i in range(colCount):
        if (i == 0):
            valString = "i"
        if (i == 1):
            val = "i%{0}".format(numOfPartitions)
            valString = valString + "," + str(val)
            continue
        if (i < colCount and i != 0):
            valString = valString + "," + "i"
    
    insertlist = []
    for inserts in insertIntos:
        insertstmt = "{0} {1} FROM generate_series(1,10000)i;".format(inserts, valString)
        insertlist.append(insertstmt)
    with open(outputFile, "a") as fh:
        for ddl in ddls:
            fh.write(ddl + "\n")
        for insertl in insertlist:
            fh.write(insertl + "\n")
        for index in indexStmt:
            fh.write(index + "\n")




if __name__ == "__main__":
    tableTypesToPrefix = {
        "heap": "heap",
        "partitionedHeap": "pheap",
        "partitionedAO": "pao",
        "partitionedAOCO": "paoco",
    }

    parser = argparse.ArgumentParser(description='Generate Tables Schema')
    parser.add_argument('-tableType', action="store", dest="tableType", type=str, choices=tableTypesToPrefix.keys(), default='partitionedAOCO', required=False)
    parser.add_argument('-numOfTables', action="store", dest="numOfTables", type=int, required=True)
    parser.add_argument('-numOfPartitions', action="store", dest="numOfPartitions", type=int)
    parser.add_argument('-numOfCols', action="store", dest="numOfCols", type=int, required=True)
    parser.add_argument('-dataType', action="store", dest="dataType", type=str, required=True)
    parser.add_argument('-indexes', action="store_true", dest="indexes", default=False, required=False)
    parser.add_argument('-outputFile', action="store", dest="outputFile", type=str)

    args = parser.parse_args()
   
    with open(args.outputFile, "w") as fh:
        fh.truncate(0)
    genCreateStmt(tableTypesToPrefix[args.tableType], args.numOfTables, args.numOfPartitions, args.numOfCols, args.dataType, args.tableType, args.indexes, args.outputFile) 

