// SPDX-License-Identifier: MIT
pragma solidity ^0.8.7;

// NOTE: this is just for the demo and not tested, use on your own risk.
// SOURCE: https://github.com/FindoraNetwork/frc20-demo/blob/main/contracts/FRC20Token.sol

contract FRC20Token {
    string public name = "FRC20 Demo";
    string public symbol = "DEMO";
    uint8 public decimals = 6;
    
    event Transfer(address indexed _from, address indexed _to, uint _value);
    event Approval(address indexed _owner, address indexed _spender, uint _value);

    mapping (address => uint) balances;
    mapping (address => mapping (address => uint)) allowed;
    
    uint256 public totalSupply;

    function balanceOf(address _owner) public view returns (uint balance) {
        return balances[_owner];
    }

    function allowance(address _owner, address _spender) public view returns (uint remaining) {
        return allowed[_owner][_spender];
    }

    function approve(address _spender, uint _value) public returns (bool) {
        require((_value == 0) || (allowed[msg.sender][_spender] == 0));
        require(_value <= balances[msg.sender]);
        allowed[msg.sender][_spender] = _value;
        emit Approval(msg.sender, _spender, _value);
        return true;
    }

    function transfer(address _to, uint _value) public returns (bool success) {
        _transferFrom(msg.sender, _to, _value);
        return true;
    }

    function transferFrom(address _from, address _to, uint _value) public returns (bool) {
        // TODO: Revert _value if we have some problems with transfer
        allowed[_from][msg.sender] -= _value;
        _transferFrom(_from, _to, _value);
        return true;
    }

    function _transferFrom(address _from, address _to, uint _value) internal {
        require(_to != address(0)); 
        require(_value > 0);
        balances[_from] -= _value;
        balances[_to] += _value;
        emit Transfer(_from, _to, _value);
    }
    
    function mint(address _to, uint256 _value) public returns (bool) {
        require(_to != address(0)); 
        require(_value > 0);
        balances[_to] += _value;
        totalSupply += _value;
        emit Transfer(address(0), _to, _value);
        return true;
    }
}
