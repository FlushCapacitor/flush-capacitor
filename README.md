# flush-capacitor #

An online toilet occupancy indicator for Rapsberry Pi written in Go.

## API ##

* `http://$CANONICAL_URL/api/sensors` returns the current toilet states.
* `ws://$CANONICAL_RUL/changes` can be used to get real-time updates on the toilet states.

## License ##

`MIT`
