#!/usr/bin/env python3
"""Required transport/fleet regressions in a disposable, socket-only PostgreSQL.

Run as the unprivileged release builder. Never connects to a supplied database.
A successful `go test` exit alone is insufficient: every named test must pass
and no selected test or subtest may be skipped.
"""
import json
import gzip
import hashlib
import urllib.request
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from urllib.parse import urlencode

REQUIRED = {
    "internal/cloudflare": ("TestCertificateNodeOwnershipAtomicInstallAndPrivateKeyRetention","TestOriginCertificateTrustNamesExpiryAndCSRBinding","TestCloudflareDriverPaginationAuthAndRedaction","TestCloudflareAmbiguousWriteIsNotRetriedByDriver","TestCloudflareNamesLifetimeAndOwnership"),
    "internal/worker": ("TestOwnedDeferralPreservesFailuresAndReplacementClaim",),
    "cmd/worker": ("TestProofBusyIsDeferredWithoutFailure","TestControlSchedulerIndependentFromBulk","TestIdentityProviderBulkPostgresPoolHeadroom","TestProofPoolFortyPhysicalPinsAndProgress"),
    "internal/panels/configdriver": ("TestCloudflareTransportPortsAndStrictOutput","TestCloudflareNativeOutboundSyntax","TestDriverCompositionAndOwnership","TestNativeConfigDriverRoundTrips","TestResizePreservesIndependentIdentities","TestClassPoliciesPreserveSiblingIdentityLifetimeAndRate","TestClassOutputFlowMaskAndCertificateOptions"),
    "internal/panels/configplane": ("TestCloudflareCertificateDriverTransition","TestTrafficProofRetriesTransientFailuresWithoutWeakeningTLS","TestBatchProofRejectsMixedOrIncompleteResponses","TestRuntimeBatchExecutesChecksAndIsolatesFailures","TestProfileSNISelectionRequiresVerifiedPublicTLS"),
    "internal/provisioning": ("TestSSHInputUsesPinnedEncryptedStdin",),
    "internal/panels/sanaei": ("TestLocalRouteBatchReadOnlyAndComplete",),
    "internal/panels/residentialsync": ("TestProofRechecksDueAfterQueuedOwnership","TestGeneratedTrojanPassesStrictResidentialSniffing","TestNativeRouteBatchPreservesCompleteMatrix","TestGoogleAdsSelectiveScopeAndRollback","TestGoogleAdsSelectiveRoutingAndProxyOutage","TestInstalledCoreGoogleAdsSelectiveTrafficAndOutage"),
    "internal/adminapi": (
        "TestCloudflareCertificateLostResponseRenewalAndNodeRepair",
        "TestCloudflareDNSCleanupSurvivesPanelPurge",
        "TestCloudflareLifecycleLostResponsesAndCleanupWithoutZones",
        "TestCloudflareForeignRecordIsNeverDeleted",
        "TestCloudflareSecretsIsolationAndKeyVersions",
        "TestCloudflareStatusNeverReturnsCredentials",
        "TestCloudflareOutputRevocationAndRecordOwnership",
        "TestCloudflareOperatorMeasurementRangeFreshnessAndIdempotency",
        "TestUnifiedProfileResizeDeleteAndRecovery",
        "TestUnifiedProfileCreationIdempotency",
        "TestUnifiedAuthorityRetiresLegacyWithoutBlockingCompletedPanels",
        "TestUnifiedStatusCountsActualPublishedOutput",
        "TestProfileOutputRevisionScopeAndCountries",
        "TestConfigProfileAdoptionPreservesNativeCredentials",
        "TestConfigDriverProfileAdmission",
        "TestConfigDriverRecoveryAndIndependentPortChange",
        "TestTransportApplySaveIsAtomicAndScopesEligibleListeners",
        "TestOutputCountryNamesAllRoutesAndObservedRegion",
        "TestTrojanMixedOutputKeepsRealityAndScope",
        "TestTrojanPolicyPortsCleanupAndRealityPreservation",
        "TestTrojanCreateLostResponseAndRestart",
        "TestTransportApplyExcludesUnfinishedCleanup",
        "TestTransportRollbackCannotCrossConfirmedCleanup",
        "TestDurableCleanupLostResponseAndWorkerRestart",
        "TestCapacityCoverageRetainsStaleAndMissingInbounds",
        "TestClassOutputDisjointImmutableAndFresh",
        "TestProfileOutputExcludesManualAndOtherOwnershipClass",
        "TestProfilePortFilterIncludesIPv6AndRejectsRemovedPorts",
        "TestOutputConfigTryLockOwnershipAndCleanup",
    ),
    "internal/panels/usercapacity": (
        "TestFleetCoverageBeyond64PreservesExecutionAndScopeGuards",
    ),
}


def verify_events(lines, required=REQUIRED):
    expected = {(package, test) for package, tests in required.items() for test in tests}
    passed = set()
    for line in lines:
        try:
            event = json.loads(line)
        except json.JSONDecodeError as exc:
            raise RuntimeError("invalid test-event stream") from exc
        package = event.get("Package", "")
        package = next((p for p in required if package == p or package.endswith("/"+p)), package)
        test = event.get("Test", "")
        root = test.split("/", 1)[0]
        if event.get("Action") == "fail":
            raise RuntimeError("regression failed: " + package + "/" + test)
        if (package, root) in expected and event.get("Action") == "skip":
            raise RuntimeError("required regression skipped: " + package + "/" + test)
        if (package, test) in expected and event.get("Action") == "pass":
            passed.add((package, test))
    missing = expected - passed
    if missing:
        raise RuntimeError("required regressions did not pass: " + ", ".join(t for _, t in sorted(missing)))
    return len(passed)


def main(repo):
    if os.geteuid() == 0:
        raise RuntimeError("gate must run as the unprivileged release builder")
    repo = Path(repo).resolve(strict=True)
    if not (repo / "go.mod").is_file():
        raise RuntimeError("candidate source required")
    candidates = [p for p in Path('/usr/lib/postgresql').glob('*/bin')
                  if p.parent.name.isdigit() and all((p / name).is_file() for name in ('initdb', 'pg_ctl', 'psql', 'createdb'))]
    if not candidates:
        raise RuntimeError("isolated PostgreSQL binaries unavailable; publication refused")
    pg = max(candidates, key=lambda p: int(p.parent.name))
    work = Path(tempfile.mkdtemp(prefix='dob-transport-gate-'))
    data, socket = work / 'data', work / 'socket'
    socket.mkdir(mode=0o700)
    env = {k: v for k, v in os.environ.items()
           if not k.startswith('PG') and k not in ('DATABASE_URL', 'BULK_TEST_DATABASE_URL', 'DOB_E2E_DSN')}
    env.update(PGHOST=str(socket), PGPORT='55435', PGUSER='dob_gate', PGDATABASE='postgres', PGCONNECT_TIMEOUT='5')
    log = work / 'setup.log'
    def run(args, **kwargs):
        return subprocess.run([str(x) for x in args], check=True, env=env, timeout=60, **kwargs)
    try:
        with log.open('w') as output:
            run([pg / 'initdb', '-D', data, '-U', 'dob_gate', '--auth-local=trust', '--auth-host=reject', '--no-instructions'], stdout=output, stderr=subprocess.STDOUT)
            with (data / 'postgresql.conf').open('a') as conf:
                conf.write("\nlisten_addresses = ''\nunix_socket_directories = '" + str(socket) + "'\nunix_socket_permissions = 0700\nport = 55435\nmax_connections = 50\n")
            run([pg / 'pg_ctl', '-D', data, '-l', work / 'postgres.log', '-w', 'start'], stdout=output, stderr=subprocess.STDOUT)
            actual = run([pg / 'psql', '-XAt', '-c', "SELECT current_setting('data_directory') || '|' || current_setting('listen_addresses')"], capture_output=True, text=True).stdout.strip()
            if actual != str(data) + '|':
                raise RuntimeError('isolated database identity check failed')
            run([pg / 'createdb', 'bulk_test_transport'], stdout=output, stderr=subprocess.STDOUT)
            run([pg / 'psql', '-X', '-v', 'ON_ERROR_STOP=1', '-c', 'CREATE ROLE digitaloceanbot; CREATE ROLE digitaloceanbot_readonly;'], stdout=output, stderr=subprocess.STDOUT)
        env['BULK_TEST_DATABASE_URL'] = 'postgresql://dob_gate@/bulk_test_transport?' + urlencode({'host': str(socket), 'port': '55435', 'sslmode': 'disable'})
        # Execute protocol/transport exports against the exact release-pinned core.
        # Neither an absent binary nor an environment-selected unverified core can skip this gate.
        core = json.loads((repo/'internal/panels/transportcore/manifest.json').read_text())['core']
        packed = (repo/'xray.gz')
        payload = packed.read_bytes() if packed.is_file() else urllib.request.urlopen(core['url'], timeout=45).read(100_000_001)
        if len(payload)>100_000_000 or hashlib.sha256(payload).hexdigest()!=core['gzip_sha256']:
            raise RuntimeError('native test core archive hash mismatch')
        binary=gzip.decompress(payload)
        if hashlib.sha256(binary).hexdigest()!=core['sha256']:
            raise RuntimeError('native test core binary hash mismatch')
        native=work/'xray';native.write_bytes(binary);native.chmod(0o700)
        env['XRAY_TEST_BINARY']=str(native)
        names = [name for group in REQUIRED.values() for name in group]
        args = ['go', 'test', '-json', '-race', '-count=1', '-p', '1', '-timeout', '5m', '-run', '^(' + '|'.join(names) + ')$'] + ['./' + p for p in REQUIRED]
        result = subprocess.run(args, cwd=repo, env=env, capture_output=True, text=True, timeout=900)
        # Preserve individual test results in the publisher's candidate evidence.
        print(result.stdout, end='', flush=True)
        if result.stderr:
            print(result.stderr, file=sys.stderr, end='', flush=True)
        if result.returncode:
            raise RuntimeError('database/race regression command failed')
        count = verify_events(result.stdout.splitlines())
        print(json.dumps({'gate': 'transport_and_fleet_regressions', 'required_passed': count, 'skipped': 0, 'database': 'disposable_private_socket_only'}), flush=True)
    finally:
        if (data / 'postmaster.pid').exists():
            # If shutdown cannot be proved, fail and retain the private directory.
            run([pg / 'pg_ctl', '-D', data, '-m', 'fast', '-w', 'stop'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        shutil.rmtree(work)


if __name__ == '__main__':
    try:
        if len(sys.argv) != 2:
            raise RuntimeError('usage: transport-regression-gate.py CANDIDATE')
        main(sys.argv[1])
    except Exception as exc:
        print('Transport regression gate blocked: ' + (str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__), file=sys.stderr)
        sys.exit(1)
