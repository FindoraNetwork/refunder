# end to end tests

### pre-install
- docker or podman, need a container runtime
- just ( https://github.com/casey/just )

references:

https://docs.soliditylang.org/en/latest/installing-solidity.html

### generating golang code from solidity
```bash
just gen
```

### testing environment parameters

those end-to-end tests are required environment parameters as input and those are been set up in the GitHub action.

- PRIVATE_KEY

