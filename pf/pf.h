/**
 * Created: Oct 25, 2020.
 * License: MIT.
 */

#pragma once

int pf_open(int);
int pf_close(int);
int pf_is_enabled(int);
int pf_add_addr(int, const char *, const char *, const void *, size_t);
int pf_add_table(int, const char *, const char *);
