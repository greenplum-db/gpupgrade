# gpupgrade command-line interface(cli)

See the accompanying ```README.md``` for more context on developing for gpugrade.
This document discusses gpupgrade from the standpoint of someone using gpupgrade to
actually upgrade their cluster.

### Summary

We run the following commands.  Each command prints some status information, and you can see the state 
of the conversion by running ```gpupgrade status upgrade```.

1. ```gpupgrade prepare init --old-bindir OLDDIR --new-bindir NEWDIR```

1. ```gpupgrade prepare start-hub```

1. ```gpupgrade check config```

1. ```gpupgrade prepare start-agents```

1. ```gpupgrade prepare init-cluster```

1. ```gpupgrade prepare shutdown-clusters```

1. ```gpupgrade upgrade convert-master```

1. ```gpupgrade upgrade share-oids```

1. ```gpupgrade upgrade convert-primaries```

1. ```gpupgrade upgrade validate-start-cluster```

1. ```gpupgrade upgrade reconfigure-ports```

###TODO

1. why is ```Install binaries on segments``` always pending?

### Details

Shown here is the result of running each command, as well as the output of running
```gpupgrade status config``` after each command.

1 ```gpupgrade prepare init --old-bindir OLDDIR --new-bindir NEWDIR```


No output is given; you specify the old and new binary directories for gpdb.  If you attempt
to check the status of the upgrade, it will fail as the status reporting requires the agent:

   ```
   → gpupgrade prepare init --old-bindir /usr/local/gpdb/bin --new-bindir /usr/local/gpdbNEW/bin
   → gpupgrade status upgrade
   [ERROR]:-couldn't connect to the upgrade hub (did you run 'gpupgrade prepare start-hub'?)
   ```
   
   ```
   (optional)
   → gpupgrade config show
   new-bindir - /usr/local/gpdbErasemeU/bin
   old-bindir - /usr/local/gpdbEraseme/bin
   ```
   
2 ```gpupgrade prepare start-hub```

No output is given; this starts the hub on the master.  At this point, you can get status
information

```
→ gpupgrade prepare start-hub

→ gpupgrade status upgrade
PENDING - Configuration Check
PENDING - Install binaries on segments
PENDING - Agents Started on Cluster
PENDING - Initialize new cluster
PENDING - Shutdown clusters
PENDING - Run pg_upgrade on master
PENDING - Copy OID files from master to segments 
COMPLETE - Run pg_upgrade on primaries                        <----should be PENDING
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

3 ```gpupgrade check config```

```
→ gpupgrade check config
[INFO]:-Check config request is processed.

→ gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
PENDING - Agents Started on Cluster
PENDING - Initialize new cluster
PENDING - Shutdown clusters
PENDING - Run pg_upgrade on master
PENDING - Copy OID files from master to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

4 ```gpupgrade prepare start-agents```

```
 → gpupgrade prepare start-agents
[INFO]:-Started Agents in progress, check gpupgrade_agent logs for details

→ gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
PENDING - Initialize new cluster
PENDING - Shutdown clusters
PENDING - Run pg_upgrade on master
PENDING - Copy OID files from master to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports

```

5 ```gpupgrade prepare init-cluster```

```
 → gpupgrade prepare init-cluster
 [INFO]:-Starting new cluster initialization

 → gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
COMLETE - Initialize new cluster
PENDING - Shutdown clusters
PENDING - Run pg_upgrade on master
PENDING - Copy OID files from master to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

6 ```gpupgrade prepare shutdown-clusters```

```
 → gpupgrade prepare shutdown-clusters
INFO]:-request to shutdown clusters sent to hub

 → gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
PENDING - Run pg_upgrade on master
PENDING - Copy OID files from master to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

7 ```gpupgrade upgrade convert-master```

```
 → gpupgrade upgrade convert-master
[INFO]:-Kicked off pg_upgrade request.

 → gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
PENDING - Copy OID files from master to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

8 ```gpupgrade upgrade share-oids```

```
 → gpupgrade upgrade share-oids
[INFO]:-Kicked off request to share oids

 → gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy OID files from master to segments
PENDING - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

9 ```gpupgrade upgrade convert-primaries```

```
 → gpupgrade upgrade convert-primaries
INFO]:-Kicked off pg_upgrade request for primaries

 → gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy OID files from master to segments
COMLETE - Run pg_upgrade on primaries
PENDING - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

10 ```gpupgrade upgrade validate-start-cluster```

```
 → gpupgrade upgrade validate-start-cluster
[INFO]:-Kicked off request for validation of cluster startup

 → gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy OID files from master to segments
COMPLETE - Run pg_upgrade on primaries
COMPLETE - Validate the upgraded cluster can start up
PENDING - Adjust upgraded cluster ports
```

11 ```gpupgrade upgrade reconfigure-ports```

```
 → gpupgrade upgrade reconfigure-ports
[INFO]:-Request to reconfigure master port on upgraded cluster complete

 → gpupgrade status upgrade
COMPLETE - Configuration Check
PENDING - Install binaries on segments
COMPLETE - Agents Started on Cluster
COMPLETE - Initialize new cluster
COMPLETE - Shutdown clusters
COMPLETE - Run pg_upgrade on master
COMPLETE - Copy OID files from master to segments
COMPLETE - Run pg_upgrade on primaries
COMPLETE - Validate the upgraded cluster can start up
COMPLETE - Adjust upgraded cluster ports
```







