# Type 2: Gas Refund

- triggered on UTC+0 specific HH:MM time everyday
- User will be refunded once only
- User to transfer more than a threshold of USDT amount from BSC or Ethereum
- Pick Min price from the previous day, No need for reducing factor.
    - the lowest BNB_USTD price
    - the highest FRA_USTD price
    - actions:
        1. get bnb/usdt, fra/usdt, btc/usdt, eth/usdt price from [gate.io](http://gate.io/) regularly (every 10 mins)
        2. every 24 hours get transfer to address value more than 50 usd
        3. send him XXX * price(bnb/usdt) / price(fra/usdt) FRA
        4. XXX = 12000000000000000 wei or a fixed wei
