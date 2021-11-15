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
Prefix: /usr/local/bin
Requires: openssh rsync >= 3.0

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
mkdir -p %{buildroot}%{prefix}
mv gpupgrade %{buildroot}%{prefix}
mv data-migration-scripts/gpupgrade-migration-sql-generator.bash %{buildroot}%{prefix}/gpupgrade-migration-sql-generator.bash
mv data-migration-scripts/gpupgrade-migration-sql-executor.bash %{buildroot}%{prefix}/gpupgrade-migration-sql-executor.bash

# additional files
mkdir -p %{buildroot}%{prefix}/greenplum/%{name}
mv data-migration-scripts %{buildroot}%{prefix}/greenplum/%{name}
mv gpupgrade_config %{buildroot}%{prefix}/greenplum/%{name}
mv gpupgrade.bash %{buildroot}%{prefix}/greenplum/%{name}
mv open_source_licenses.txt %{buildroot}%{prefix}/greenplum/%{name}


%files
# executables
%{prefix}/gpupgrade
%{prefix}/gpupgrade-migration-sql-generator.bash
%{prefix}/gpupgrade-migration-sql-executor.bash

# additional files
%dir %{prefix}/greenplum
%dir %{prefix}/greenplum/%{name}
%{prefix}/greenplum/%{name}/data-migration-scripts
%config %{prefix}/greenplum/%{name}/gpupgrade_config
%{prefix}/greenplum/%{name}/gpupgrade.bash
%{prefix}/greenplum/%{name}/open_source_licenses.txt
