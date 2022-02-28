# Type 2: Gas Refund

- triggered on 00:00 UTC+0
- User has to wait for 1 day
- User to transfer more than 200 USD from BSC or Ethereum
- Pick Min price from the previous day, No need for reducing factor.
    - the lowest BNB_USTD price
    - the highest FRA_USTD price
    - actions:
        1. get bnb/usdt, fra/usdt, btc/usdt, eth/usdt price from [gate.io](http://gate.io/) regularly (every 10 mins)
        2. every 24 hours get transfer to address value more than 200 usd
        3. send him XXX * price(bnb/usdt) / price(fra/usdt) FRA
        4. XXX = 0**.**00053251 * 0.5 (**2.66255e+14 wei**)

Max Cap :  
- A fixed threshold
