Name: gpupgrade
Version: %{gpupgrade_version}
# Release is a way of versioning the spec file.
# Only bump the Release if shipping gpupgrade without also bumping the
# gpugprade_version (ie: VERSION).
Release: %{gpupgrade_rpm_release}%{?dist}
Summary: %{summary}
License: %{license}
URL: https://github.com/greenplum-db/gpupgrade
Source0: %{name}.tar.gz
# Allow the RPM to be relocatable by setting prefix to "/".
Prefix: /
Requires: openssh rsync

%description
The gpupgrade package contains gpupgrade which performs in-place upgrades
without the need for additional hardware, disk space, and with less downtime.

%prep
# If the gpupgrade_version macro is not defined, it gets interpreted as a
# literal string, use %% to escape it
if [ %{gpupgrade_version} = '%%{gpupgrade_version}' ] ; then
    echo "The macro (variable) gpupgrade_version must be supplied as rpmbuild ... --define='gpupgrade_version [VERSION]'"
    exit 1
fi

%setup -q -c -n %{name}

%install
# executables
mkdir -p %{buildroot}%{prefix}/usr/local/bin
mv gpupgrade %{buildroot}%{prefix}/usr/local/bin
mv data-migration-scripts/migration_generator_sql.bash  %{buildroot}%{prefix}/usr/local/bin/gpupgrade-migration-sql-generator.bash
mv data-migration-scripts/migration_executor_sql.bash  %{buildroot}%{prefix}/usr/local/bin/gpupgrade-migration-sql-executor.bash

# scripts
mkdir -p %{buildroot}%{prefix}/usr/local/bin/greenplum/%{name}
mv data-migration-scripts %{buildroot}%{prefix}/usr/local/bin/greenplum/%{name}

# config
mkdir -p %{buildroot}%{prefix}%{_sysconfdir}/greenplum/%{name}
mv gpupgrade_config %{buildroot}%{prefix}%{_sysconfdir}/greenplum/%{name}

# bash_completion
mkdir -p %{buildroot}%{prefix}%{_sysconfdir}/bash_completion.d
mv gpupgrade.bash %{buildroot}%{prefix}%{_sysconfdir}/bash_completion.d

# license
mkdir -p %{buildroot}%{prefix}/usr/share/doc
mkdir -p %{buildroot}%{prefix}/usr/share/licenses

%files
# executables
%dir %{prefix}/usr
%dir %{prefix}/usr/local
%dir %{prefix}/usr/local/bin
%{prefix}/usr/local/bin/gpupgrade
%{prefix}/usr/local/bin/gpupgrade-migration-sql-generator.bash
%{prefix}/usr/local/bin/gpupgrade-migration-sql-executor.bash

# scripts
%dir %{prefix}/usr/local/bin/greenplum
%dir %{prefix}/usr/local/bin/greenplum/%{name}
%{prefix}/usr/local/bin/greenplum/%{name}/data-migration-scripts

# config
%dir %{prefix}%{_sysconfdir}
%dir %{prefix}%{_sysconfdir}/greenplum
%dir %{prefix}%{_sysconfdir}/greenplum/%{name}
%config %{prefix}%{_sysconfdir}/greenplum/%{name}/gpupgrade_config

# bash_completion
%dir %{prefix}%{_sysconfdir}/bash_completion.d
%{prefix}%{_sysconfdir}/bash_completion.d/gpupgrade.bash

# license
%dir %{prefix}/usr/share
%dir %{prefix}/usr/share/doc
%dir %{prefix}/usr/share/licenses
# Define the license macro to work for both centos 6 and 7.
# For centos 6: /usr/share/doc/gpupgrade-0.4.0/open_source_licenses.txt
# For centos 7: /usr/share/licenses/gpupgrade-0.4.0/open_source_licenses.txt
# See: https://access.redhat.com/documentation/en-us/red_hat_software_collections/3/html/packaging_guide/sect-differences_between_red_hat_enterprise_linux_6_and_7
%{!?_licensedir:%global license %%doc}
%license open_source_licenses.txt
