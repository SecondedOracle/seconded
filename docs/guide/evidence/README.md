# Public catalog captures

Unauthenticated GET responses collected on 2026-10-06 from
[the public products endpoint](https://api.secondedoracle.xyz/v1/products).
Advertisement is not a successful purchase or independent settlement test.

| File | Request header | Response-body SHA-256 |
| --- | --- | --- |
| [products-client-0.4.1-2026-10-06.json](products-client-0.4.1-2026-10-06.json) | `SECONDED-CLIENT-VERSION: 0.4.1` | `2d562e4d603a93d5ee2e509eab1c801996358411881db4e3dd47b99fec5773ca` |
| [products-unversioned-2026-10-06.json](products-unversioned-2026-10-06.json) | none | `ab2a0bc99fad44fb0d4d7ff2facb30bd56060a2703ee48e978f19f8d8b51c8ed` |

Repeat the versioned observation:

```sh
curl -fsS -H 'SECONDED-CLIENT-VERSION: 0.4.1' \
  https://api.secondedoracle.xyz/v1/products
```

The unversioned request advertised eight checks; the versioned request advertised
twelve. `client/products-live-v1.json` remains the unmodified September 30 capture.
These captures contain only public product/network metadata.

Captured vendor `usage` and coverage wording may exceed the guarantees verified here.
These exact historical responses are evidence of advertisement, not endorsements of
every sentence. Read the retention and pending-billing qualifications in [CLAIMS](../../CLAIMS.md).
