# gpupgrade command-line interface(cli)

See the accompanying ```README.md``` for more context on developing for gpugrade.
This document discusses gpupgrade from the standpoint of someone using gpupgrade to
actually upgrade their cluster.

### Summary

We run the following commands.  Each command prints some status information, and you can see the state 
of the conversion by running ```gpupgrade status upgrade```.

1. ```gpupgrade initialize --old-bindir OLDDIR --new-bindir NEWDIR --old-port OLDPORT```

1. ```gpupgrade prepare init-cluster```

1. ```gpupgrade prepare shutdown-clusters```

1. ```gpupgrade upgrade convert-master```

1. ```gpupgrade upgrade copy-master```

1. ```gpupgrade upgrade convert-primaries```

1. ```gpupgrade upgrade validate-start-cluster```

1. ```gpupgrade upgrade reconfigure-ports```

###TODO

1. why is ```Install binaries on segments``` always pending?

### Details

Shown here is the result of running each command, as well as the output of running
```gpupgrade status config``` after each command.

1 ```gpupgrade initialize --old-bindir OLDDIR --new-bindir NEWDIR --old-port OLDPORT```


No output is given; you specify the old and new binary directories for gpdb.  If you attempt
to check the status of the upgrade, it will fail as the status reporting requires the agent:

   ```
   → gpupgrade initialize --old-bindir "${GPHOME}/bin" --new-bindir "${GPHOME}/bin" --old-port ${PGPORT}
   gpupgrade prepare init --old-bindir /usr/local/gpdb/bin --new-bindir /usr/local/gpdbNEW/bin
   → gpupgrade status upgrade
   gpupgrade status upgrade
   COMPLETE - Configuration Check
   COMPLETE - Agents Started on Cluster
   PENDING - Initialize new cluster
   PENDING - Shutdown clusters
   PENDING - Run pg_upgrade on master
   PENDING - Copy master data directory to segments
   PENDING - Run pg_upgrade on primaries
   PENDING - Validate the upgraded cluster can start up
   PENDING - Adjust upgraded cluster ports
   ```
   
   ```
   (optional)
   → gpupgrade config show
   new-bindir - /usr/local/gpdb/bin
   old-bindir - /usr/local/gpdb/bin
   ```

2 ```gpupgrade prepare init-cluster```

```
 → gpupgrade prepare init-cluster
 [INFO]:-Starting new cluster initialization

 → gpupgrade status upgrade
COMPLETE - Configuration Check
COMPLETE - Agents Started on Cluster
COMLETE - Initialize new cluster
PENDING - Shutdown clusters
PENDING - Run pg_upgrade on master
PENDING - Copy master data directory to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

3 ```gpupgrade prepare shutdown-clusters```

```
 → gpupgrade prepare shutdown-clusters
INFO]:-request to shutdown clusters sent to hub

 → gpupgrade status upgrade
COMPLETE - Configuration Check
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
PENDING - Run pg_upgrade on master
PENDING - Copy master data directory to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

4 ```gpupgrade upgrade convert-master```

```
 → gpupgrade upgrade convert-master
[INFO]:-Kicked off pg_upgrade request.

 → gpupgrade status upgrade
COMPLETE - Configuration Check
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
PENDING - Copy master data directory to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

5 ```gpupgrade upgrade copy-master```

```
 → gpupgrade upgrade copy-master
[INFO]:-Kicked off request to copy master

 → gpupgrade status upgrade
COMPLETE - Configuration Check
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy master data directory to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

6 ```gpupgrade upgrade convert-primaries```

```
 → gpupgrade upgrade convert-primaries
INFO]:-Kicked off pg_upgrade request for primaries

 → gpupgrade status upgrade
COMPLETE - Configuration Check
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy master data directory to segments
COMLETE - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

7 ```gpupgrade upgrade validate-start-cluster```

```
 → gpupgrade upgrade validate-start-cluster
[INFO]:-Kicked off request for validation of cluster startup

 → gpupgrade status upgrade
COMPLETE - Configuration Check
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy master data directory to segments
COMPLETE - Run pg_upgrade on primaries
COMPLETE - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

8 ```gpupgrade upgrade reconfigure-ports```

```
 → gpupgrade upgrade reconfigure-ports
[INFO]:-Request to reconfigure master port on upgraded cluster complete

 → gpupgrade status upgrade
COMPLETE - Configuration Check
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy master data directory to segments
COMPLETE - Run pg_upgrade on primaries
COMPLETE - Validate the upgraded cluster can start up
COMPLETE - Adjust upgraded cluster ports
```







