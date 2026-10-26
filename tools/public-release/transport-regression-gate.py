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
    "internal/scheduler": ("TestSchedulerClaimCompletionOwnershipPostgres", "TestSchedulerPartialClaimReadRollsBackPostgres", "TestSchedulerReadAndCompletionFaultsFailClosedPostgres", "TestSchedulerCancellationReplacementAndMigrationPostgres"),
    "internal/migrate": ("TestMigrationLegacyChecksumBackfillAndDrift",),
    "internal/observability": ("TestIntegrityReportCannotHideNewFailure", "TestIntegrityReportCannotHideRecoveryOrExpiredBackup"),
    "internal/secrets": ("TestAuthorizationSecretsOwnerBindingAndRotation",),
    "internal/cloudflareschedule": ("TestCloudflareScheduleValidation",),
    "internal/cloudflare": ("TestCloudflareCertificateWireDatesAndPagination","TestCloudflareIncompleteCertificateListCannotProveAbsence","TestCloudflarePermissionErrorsIdentifyOperationWithoutSecrets","TestCertificateNodeOwnershipAtomicInstallAndPrivateKeyRetention","TestOriginCertificateTrustNamesExpiryAndCSRBinding","TestCloudflareDriverPaginationAuthAndRedaction","TestCloudflareAmbiguousWriteIsNotRetriedByDriver","TestCloudflareNamesLifetimeAndOwnership"),
    "internal/worker": ("TestOwnedDeferralPreservesFailuresAndReplacementClaim",),
    "cmd/worker": ("TestProofBusyIsDeferredWithoutFailure","TestControlSchedulerIndependentFromBulk","TestIdentityProviderBulkPostgresPoolHeadroom","TestProofPoolFortyPhysicalPinsAndProgress"),
    "internal/panels/configdriver": ("TestTCPHTTPTransportRoundTripAndValidation","TestCloudflareTransportPortsAndStrictOutput","TestCloudflareNativeOutboundSyntax","TestDriverCompositionAndOwnership","TestNativeConfigDriverRoundTrips","TestResizePreservesIndependentIdentities","TestClassPoliciesPreserveSiblingIdentityLifetimeAndRate","TestClassOutputFlowMaskAndCertificateOptions"),
    "internal/panels/configplane": ("TestProfileClaimsDoNotDiscardIndependentDestinations","TestPendingPortReservationIsPanelScoped","TestCloudflareCertificateDriverTransition","TestTrafficProofRetriesTransientFailuresWithoutWeakeningTLS","TestBatchProofRejectsMixedOrIncompleteResponses","TestRuntimeBatchExecutesChecksAndIsolatesFailures","TestProfileSNISelectionRequiresVerifiedPublicTLS"),
    "internal/provisioning": ("TestSSHInputUsesPinnedEncryptedStdin",),
    "internal/panels/sanaei": ("TestLocalRouteBatchReadOnlyAndComplete",),
    "internal/panels/residentialsync": ("TestProofRechecksDueAfterQueuedOwnership","TestGeneratedTrojanPassesStrictResidentialSniffing","TestNativeRouteBatchPreservesCompleteMatrix","TestGoogleAdsSelectiveScopeAndRollback","TestGoogleAdsSelectiveRoutingAndProxyOutage","TestInstalledCoreGoogleAdsSelectiveTrafficAndOutage"),
    "internal/adminapi": (
        "TestLatestAccountSnapshotBoundsHistoryReads",
        "TestProviderAuthorizationJournalConcurrencyRestartAndOwnership",
        "TestProviderAuthorizationUnknownExpiryAndRetainedKeys",
        "TestProviderAuthorizationAtomicAccountHandoff",
        "TestCloudflareManualLockWaitDoesNotReplay",
        "TestCloudflareDNSQuotaAdmissionRecoveryAndCleanup",
        "TestCloudflarePartialQuotaBatchPreservesInterval",
        "TestCloudflareDNSAccessIsolationAndRecovery",
        "TestCloudflareRetiresOnlyUnsentExpiredIntents",
        "TestCloudflareStatusSeparatesIntentsAndDNS",
        "TestCloudflareScheduleAdmissionDetailAndPersistence",
        "TestCloudflareCertificateLostResponseRenewalAndNodeRepair",
        "TestCloudflareDNSCleanupSurvivesPanelPurge",
        "TestCloudflareLifecycleLostResponsesAndCleanupWithoutZones",
        "TestCloudflareForeignRecordIsNeverDeleted",
        "TestCloudflareSecretsIsolationAndKeyVersions",
        "TestCloudflareStatusNeverReturnsCredentials",
        "TestCloudflareOutputRevocationAndRecordOwnership",
        "TestCloudflareOperatorMeasurementRangeFreshnessAndIdempotency",
        "TestUnifiedProfileResizeDeleteAndRecovery",
        "TestProfileDeleteImmediatelyRemovesLocalVisibility",
        "TestProfilePortReuseIsolatesPendingDeletion",
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

# Provider admission/cleanup contracts are mandatory alongside fleet regressions.
REQUIRED['internal/providers/aws'] = REQUIRED.get('internal/providers/aws', ()) + ('TestAWSCreateContractAndAmbiguousOutcome', 'TestAWSInventoryFailsClosed', 'TestAWSAccountCatalogAndScope', 'TestAWSDeleteOwnershipAndFenceTarget', 'TestAWSCredentialAndErrorIsolation', 'TestAWSSigningDeterministicAndSensitiveToBody', 'TestAWSExpectedAccountScopeRejectsCredentialRebind', 'TestAWSAbsenceWithoutEvidenceIsInconclusive', 'TestAWSSignatureOfficialVectors')
REQUIRED['internal/providers/ovh'] = REQUIRED.get('internal/providers/ovh', ()) + ('TestOVHCreateContractAndAmbiguity', 'TestOVHRegionalCatalogAndQuota', 'TestOVHInventoryAndOwnership', 'TestOVHClockSingleFlightAndCredentialValidation', 'TestOVHAmbiguousCreateFindsAllMatches', 'TestOVHRejectsNonOfficialUbuntuNames', 'TestOVHUnsupportedNetworkModeNeverCreates')
REQUIRED['internal/providers/cloudutil'] = REQUIRED.get('internal/providers/cloudutil', ()) + ('TestCredentialStrictAndHTTPIsolation',)
REQUIRED['internal/providers'] = REQUIRED.get('internal/providers', ()) + ('TestRegionalLaunchPinsNativeIDsAndFiltersFallback',)
REQUIRED['internal/droplets'] = REQUIRED.get('internal/droplets', ()) + ('TestCreateResumesOnlyUndispatchedPlannedOperation', 'TestCreateResumeConcurrentPostgres')
REQUIRED['internal/app'] = REQUIRED.get('internal/app', ()) + ('TestAWSTerminationPreservesDisabledAccountCleanupFence', 'TestOVHDeletionRequiresExactLocalProjectBinding')
REQUIRED['internal/adminapi'] = REQUIRED.get('internal/adminapi', ()) + ('TestNewCloudInventoryOwnershipAndIncompleteSnapshot', 'TestNewCloudCapacityAndDashboardCount', 'TestNewCloudAbsenceEvidenceSurvivesRestart')


# Automatic onboarding and account network resource recovery.
REQUIRED['internal/providers'] += ('TestDiscoveryRegistryEnforcesReadOnlyAndSecretBoundary',)
REQUIRED['internal/providers/aws'] += ('TestAWSAutomaticDiscoveryReadOnlyAndPinned', 'TestAWSPublicNetworkRejectsBlockedAndPartialReads', 'TestAWSManagedNetworkUnknownOutcomeAndOwnership', 'TestAWSManagedNetworkConcurrentIntent', 'TestAWSNetworkDeletionTargetStrict')
REQUIRED['internal/providers/ovh'] += ('TestOVHAutomaticDiscoveryAndProjectChoice', 'TestOVHDiscoveryFailsClosed')
REQUIRED['internal/droplets'] += ('TestNetworkPreparationFailureDoesNotStrandCompute',)
REQUIRED['internal/adminapi'] += ('TestAccountResourceJournalConcurrencyAndRestart',)
REQUIRED['internal/app'] += ('TestAWSNetworkCleanupFenceRequiresJournalAndSettledDeletion',)

REQUIRED['internal/providers/aws'] += ('TestAWSDiscoverySelectionOverridesStaleSubnet','TestAWSNetworkReadFailureNeverDispatchesCompute')

REQUIRED['internal/providers/ovh'] += ('TestOVHErrorCodesPreserveAuthenticationAndPermission','TestOVHDiscoveryDoesNotMaskPermissionWithAnotherEndpoint','TestOVHDiagnosticRedactsRequestAndProviderData','TestOVHMalformedErrorCannotTriggerEndpointFallback')
REQUIRED['internal/adminapi'] += ('TestOVHPreviewActionableDiagnosticsAndSecretBoundary',)

REQUIRED['internal/providers/ovh'] += ('TestOVHNativeClassMessageErrorEnvelope','TestOVHNativeEnvelopeDiscoveryAndScopedPermissions','TestOVHResponseShapeNeverExposesUnknownData')
REQUIRED['internal/adminapi'] += ('TestOVHPreviewNativeEnvelopeDiagnostics',)

REQUIRED['internal/providers/ovh'] += ('TestOVHAuthorizationDelegationAndActualRights','TestOVHAuthorizationUnknownOutcomeAndURLBoundary','TestOVHAuthorizationEndpointFallbackAndExpiry')

REQUIRED['internal/providers/ovh'] += ('TestOVHAuthorizationOfficialRoutesAndDiagnostics',)
REQUIRED['internal/adminapi'] += ('TestProviderAuthorizationSafeDiagnosticSurvivesRestart',)

REQUIRED['internal/app'] += ('TestVersionedCatalogRefreshIndependentOfEconomy', 'TestVersionedCatalogHealthDoesNotFetch')
REQUIRED['internal/adminapi'] += ('TestAccountOptionsUsesActualCatalogFetch',)

REQUIRED['internal/provisioning'] += ('TestKernelMemoryGuardRootAliasRequiresMountedIdentity',)
REQUIRED['internal/adminapi'] += ('TestAccountSaveReturnsCommittedRuleReceipt','TestAccountInventoryPendingCreationIsNotForeignOrManaged','TestAccountListNullInventoryDoesNotBreakSavedStatus')

REQUIRED['internal/providers/aws'] += ('TestAWSErrorClassificationPreservesActionableCause','TestAWSNonProviderErrorNeverBlamesCredentials','TestAWSDiagnosticsAllowlistBothEnvelopes')
REQUIRED['internal/adminapi'] += ('TestAWSPreviewActionableDiagnosticsAndSecretBoundary',)

REQUIRED['internal/providers/aws'] += ('TestAWSPaginationOperationContracts','TestAWSDiscoveryFollowsRouteTablePageWithValidLimit','TestAWSUnregisteredPaginationFailsBeforeRequest')

REQUIRED['internal/provisioning'] += ('TestControllerKernelMemoryGuardLifecycle','TestControllerKernelMemoryGuardContainerBoundary')

REQUIRED['internal/providers/aws'] += ('TestAWSImageMappingCatalogAndCreate', 'TestAWSEmptyCatalogIdentifiesReadStage')
REQUIRED['internal/adminapi'] += ('TestAWSPreviewEmptyCatalogDiagnostics',)

REQUIRED['internal/providers/aws'] += ('TestAWSRegionalDiscoveryPinsIndependentNetworks','TestAWSRegionalCatalogAndLaunchRouting','TestAWSRegionalInventoryNeverReturnsPartialSuccess','TestAWSRegionalCredentialsRejectScopeChanges','TestAWSRegionalCleanupRejectsUnknownJournalBeforeIO','TestAWSRegionalCleanupResumesEachOwnedNetwork','TestAWSPriceEvidenceRejectsWrongProductAndTerms','TestAWSPricingPaginationSigningAndOptionalFailure','TestAWSPlanCompatibilityAndAvailabilityZone')

REQUIRED['internal/adminapi'] += ('TestCloudflareQuotaPrioritizesUncoveredServers',)
REQUIRED['cmd/worker'] += ('TestControlBudgetReservesProviderProgress','TestIdentityReservedCapacityWithBulkAndBothProviders','TestIdentityDeadlineStartsAfterAdmission','TestIdentityRollingSlowPeerDoesNotBlockHealthyTail')

REQUIRED['internal/adminapi'] += ('TestForwardedPanelPortPreservesGateway','TestPanelSettingsAtomicCredentials','TestPanelSettingsAddressAndAdminAuthorization')

REQUIRED['internal/worker'] += ('TestWeightedAdmissionKeepsSmallWorkAndCleanup','TestAdmissionCancellationDoesNotAbandonOrLeakSlots')

REQUIRED['internal/worker'] += ('TestRecoveryLedgerReadFaultNeverCallsBypassOrHandler', 'TestRecoveryMissingCheckpointOrFailedPruningRefusesWork', 'TestRecoveryCompletionFaultPreservesDurableFenceAndAtomicBackoff', 'TestRecoveryLiveOwnerCrashAndSessionLoss', 'TestRecoveryConcurrentWorkersDoNotOverlapLiveHandler', 'TestRecoveryAsyncPersistenceFailureInvalidatesProgressUntilReconciled', 'TestRecoveryProgressGenerationRejectsOlderSuccess', 'TestRecoveryCancellationPersistsBeforeReleasingLane', 'TestRecoveryCheckpointMigrationRefusesUnsafeDowngrade', 'TestRecoveryFailedActiveLaneCannotRestoreReadiness', 'TestLifecycleCompletionFailureSurvivesRestartAndInvalidatesProgress', 'TestRecoveryReaperDoesNotTouchLifecycleLane', 'TestDispatcherSlowAccountDoesNotBlockOthers', 'TestPollClassCannotOccupyRoutingHeadroom', 'TestAdmissionReservesNestedSQLAndSupervisionHeadroom')

REQUIRED['internal/adminapi'] += ('TestDashboardSeparatesProvisioningFromUnverified',)

REQUIRED['internal/panels/configplane'] += ('TestCDNClassProofsPreserveIndependentEndpointTimes','TestCDNProofFencesAndStructuralFailure')
REQUIRED['internal/providers/aws'] += ('TestAWSBackgroundErrorRetainsSafeWireCause',)

REQUIRED['internal/adminapi'] += ('TestCloudflareFleetCapacityScopeAndOverlap','TestCloudflareCapacityUnknownAndStaleNeverClaimsSufficient')

REQUIRED['internal/worker'] += ('TestDispatcherForgetsCompletedAccounts', 'TestDispatcherCancellationKeepsExecutingAccountOwned', 'TestDispatcherAccountCountSurvivesSiblingCompletion')

# Bounded provider reads must not accept incomplete inventories or create IDs.
REQUIRED['internal/providers/vultr'] = ('TestCatalogPaginationUsesNativeCursors', 'TestCatalogPaginationRejectsInvalidInventory', 'TestCatalogPaginationStopsCyclesAndLimits', 'TestCatalogPaginationCancellation')
REQUIRED['internal/providers/digitalocean'] = ('TestProviderResponseReadBounds', 'TestProviderResponseRequiresSingleJSONValue', 'TestCreateResponseMissingIdentityRemainsAmbiguous')
REQUIRED['internal/droplets'] += ('TestAmbiguousCreateDoesNotAdvanceCapacitySuccess',)

REQUIRED['internal/app'] += ('TestSSHIdentityPersistsBeforeDispatchPostgres', 'TestSSHIdentityUnknownNeverRedispatchesPostgres')

REQUIRED['internal/app'] += ('TestSSHIdentityLostResponseReconcilesExactKeyPostgres', 'TestSSHIdentityRejectsForeignAndIncompleteEvidencePostgres', 'TestSSHIdentityConcurrentClaimAndStaleSnapshotPostgres', 'TestSSHIdentityReservationAtomicityAndCrashWindowsPostgres', 'TestSSHIdentityScopeAndLegacyRecoveryPostgres', 'TestSSHIdentityInlineAndPermissionFallbackPostgres')
REQUIRED['internal/providers/vultr'] += ('TestSSHImportUncertainVerificationNeverDeletes', 'TestSSHInventoryCarriesExactPublicKeyAndIsolatesCursor')
REQUIRED['internal/providers/digitalocean'] += ('TestSSHInventoryCarriesExactPublicKey',)

REQUIRED['internal/app'] += ('TestSSHIdentityStaleGenerationAndSecretCollisionPostgres',)

REQUIRED['internal/secrets'] += ('TestSecretRecordCorruptionNeverPanics','TestSecretSQLReadersRetainVersionAndRejectCorruption')

REQUIRED['internal/app'] += ('TestLifecycleDeleteConfirmationInterruptedReadRollsBack', 'TestObservationRetentionCatchupAndLimits')

REQUIRED['internal/adminapi'] += ('TestAdminListsRejectPartialDatabaseResults',)

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
        # Keep native routing tests independent of a controller's global assets.
        # Pin the upstream release and GitHub-published digest; never skip the
        # Google Ads path tests because a fresh runner lacks geosite.dat.
        geo_url='https://github.com/v2fly/domain-list-community/releases/download/20261024005255/dlc.dat'
        geo=urllib.request.urlopen(geo_url,timeout=45).read(10_000_001)
        if hashlib.sha256(geo).hexdigest()!='356b6accce7b724490ddffee25e69edc9de8a1e7fe4e799871cf5c417fdf31d7':
            raise RuntimeError('native test geosite hash mismatch')
        (work/'geosite.dat').write_bytes(geo)
        env['XRAY_LOCATION_ASSET']=str(work)

        # A prior independent run hit the existing 10s discovery deadline in
        # this isolation fixture. Exercise the unchanged deadline repeatedly;
        # never turn a failed run into success by retrying it inside the gate.
        repeat = subprocess.run(['go', 'test', '-json', '-race', '-count=20', '-p', '1',
                                 '-timeout', '4m', '-run', '^TestRecoveryReaperDoesNotTouchLifecycleLane$',
                                 './internal/worker'], cwd=repo, env=env, capture_output=True,
                                text=True, timeout=270)
        print(repeat.stdout, end='', flush=True)
        if repeat.stderr:
            print(repeat.stderr, file=sys.stderr, end='', flush=True)
        if repeat.returncode:
            raise RuntimeError('repeated recovery/lifecycle isolation regression failed')
        verify_events(repeat.stdout.splitlines(), {'internal/worker': ('TestRecoveryReaperDoesNotTouchLifecycleLane',)})
        repeat_events = [json.loads(line) for line in repeat.stdout.splitlines() if line.strip()]
        repeat_passes = sum(x.get('Test') == 'TestRecoveryReaperDoesNotTouchLifecycleLane'
                            and x.get('Action') == 'pass' for x in repeat_events)
        if repeat_passes != 20:
            raise RuntimeError('repeated recovery/lifecycle isolation evidence incomplete')
        print(json.dumps({'recovery_lifecycle_isolation_repetitions_passed': repeat_passes}), flush=True)

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
        subprocess.run(['node', 'web/tests/aws-catalog.cjs'], cwd=repo, env=env, check=True, timeout=30)
        subprocess.run(['node', 'web/tests/database-health.cjs'], cwd=repo, env=env, check=True, timeout=30)
        subprocess.run(['node', 'web/tests/account-save-receipt.cjs'], cwd=repo, env=env, check=True, timeout=30)
        subprocess.run(['node', 'web/tests/account-rule-status.cjs'], cwd=repo, env=env, check=True, timeout=30)
        subprocess.run(['node', 'web/tests/provider-authorization-urls.cjs'], cwd=repo, env=env, check=True, timeout=30)
        print(json.dumps({'gate': 'transport_and_fleet_regressions', 'required_passed': count, 'skipped': 0, 'database': 'disposable_private_socket_only'}), flush=True)
    finally:
        if (data / 'postmaster.pid').exists():
            # If shutdown cannot be proved, fail and retain the private directory.
            run([pg / 'pg_ctl', '-D', data, '-m', 'fast', '-w', 'stop'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        shutil.rmtree(work)


REQUIRED["internal/panels/sanaei"] += ("TestRuntimeFailedOpenReleasesIdentityState", "TestRuntimeAcquisitionSweepsExpiredIdentities", "TestRuntimeMutationIdentitySurvivesCacheReplacement", "TestRuntimeMutationCancellationAndPanicReleaseGate", "TestRuntimeMutationWaitHonorsDeadlineBeforeCallback", "TestRuntimeAcquisitionWaitHasPreMutationProvenance", "TestRuntimeFactorySQLFailuresNeverBecomePanelCircuit")

REQUIRED["internal/providers/vultr"] += ("TestSSHDeletionInventoryRejectsMissingIdentity",)
REQUIRED["internal/app"] += ("TestAccountSSHDeletionRequiresValidAbsenceEvidence", "TestDeletionRecoveryValidatesInventoryBeforeAdoption")

if __name__ == '__main__':
    try:
        if len(sys.argv) != 2:
            raise RuntimeError('usage: transport-regression-gate.py CANDIDATE')
        main(sys.argv[1])
    except Exception as exc:
        print('Transport regression gate blocked: ' + (str(exc) if isinstance(exc, RuntimeError) else type(exc).__name__), file=sys.stderr)
        sys.exit(1)
