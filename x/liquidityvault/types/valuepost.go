package types

// ValuePostWindow is how many value posts are retained per vault; the
// composite score uses their median. Six posts per complex-check window,
// per the LPV design document.
const ValuePostWindow = 6
