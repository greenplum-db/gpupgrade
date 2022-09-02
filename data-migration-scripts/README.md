# Data Migration Scripts

Data migration scripts are run at specific **phases** of the upgrade cycle such as initialize, finalize,and revert.
The data migration scripts are generated from **seed scripts** located here in our repo and
installed in `/usr/local/bin/greenplum/gpupgrade/data-migration-scripts`. Customers will first **generate** data
migration scripts based on their source cluster using the **seed scripts**. Next, they **execute** them depending on 
the specific **phase**.

Important Reminders:
- The reason the **generator** and **executor** is split into two stages allows customers to inspect the generated SQL 
to be run on their cluster which affects their data. That is, some customers need to see exactly what will be run on 
their data.
- The **generator** takes a "snapshot" of the current source cluster to create generated SQL migration scripts. If new 
problematic objects are added **after** the generator was first run, then the previously generated scripts are outdated.
The generator will need to be re-run to capture the newly added objects.
- All **seed scripts** used to generate the data migration scripts are executed on the **source cluster**.
- The **generated scripts** for stats, initialize, and revert are executed on the **source cluster**.
- The **generated scripts** for finalize are executed on the **target cluster**.
