'use strict';
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {validApprovalURL}=require('../static/provider-authorization.js');
const fixtures=JSON.parse(fs.readFileSync(path.join(__dirname,'../../internal/providers/ovh/testdata/authorization-urls.json'),'utf8'));
for(const f of fixtures)assert.equal(validApprovalURL(f.url),f.browser_valid,f.url);
for(const x of [null,undefined,{},42])assert.equal(validApprovalURL(x),false);
console.log('OVH_OFFICIAL_APPROVAL_URLS_AND_UNTRUSTED_BOUNDARY PASS '+fixtures.length);
